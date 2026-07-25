package openai

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func OpenaiRealtimeHandler(c *gin.Context, info *relaycommon.RelayInfo) (*types.NewAPIError, *dto.RealtimeUsage) {
	if info == nil || info.ClientWs == nil || info.TargetWs == nil {
		return types.NewError(fmt.Errorf("invalid websocket connection"), types.ErrorCodeBadResponse), nil
	}

	info.IsStream = true
	clientConn := info.ClientWs
	targetConn := info.TargetWs

	clientClosed := make(chan struct{})
	targetClosed := make(chan struct{})
	sendChan := make(chan []byte, 100)
	receiveChan := make(chan []byte, 100)
	errChan := make(chan error, 2)
	var sawResponseDone atomic.Bool
	var stopping atomic.Bool
	var usageMu sync.Mutex
	var readers sync.WaitGroup

	usage := &dto.RealtimeUsage{}
	localUsage := &dto.RealtimeUsage{}
	sumUsage := &dto.RealtimeUsage{}

	readers.Add(2)
	gopool.Go(func() {
		defer readers.Done()
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in client reader: %v", r)
			}
		}()
		for {
			select {
			case <-c.Done():
				return
			default:
				_, message, err := clientConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						if !stopping.Load() {
							errChan <- fmt.Errorf("error reading from client: %v", err)
						}
					}
					close(clientClosed)
					return
				}

				realtimeEvent := &dto.RealtimeEvent{}
				err = common.Unmarshal(message, realtimeEvent)
				if err != nil {
					errChan <- fmt.Errorf("error unmarshalling message: %v", err)
					return
				}

				if realtimeEvent.Type == dto.RealtimeEventTypeSessionUpdate {
					if realtimeEvent.Session != nil {
						if realtimeEvent.Session.Tools != nil {
							info.RealtimeTools = realtimeEvent.Session.Tools
						}
					}
				}

				textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
				if err != nil {
					errChan <- fmt.Errorf("error counting text token: %v", err)
					return
				}
				logger.LogInfo(c, fmt.Sprintf("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
				usageMu.Lock()
				localUsage.TotalTokens += textToken + audioToken
				localUsage.InputTokens += textToken + audioToken
				localUsage.InputTokenDetails.TextTokens += textToken
				localUsage.InputTokenDetails.AudioTokens += audioToken
				usageMu.Unlock()

				info.MarkAttemptDispatched()
				err = helper.WssString(c, targetConn, string(message))
				if err != nil {
					errChan <- fmt.Errorf("error writing to target: %v", err)
					return
				}

				select {
				case sendChan <- message:
				default:
				}
			}
		}
	})

	gopool.Go(func() {
		defer readers.Done()
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in target reader: %v", r)
			}
		}()
		for {
			select {
			case <-c.Done():
				return
			default:
				_, message, err := targetConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						if !stopping.Load() {
							errChan <- fmt.Errorf("error reading from target: %v", err)
						}
					}
					close(targetClosed)
					return
				}
				info.SetFirstResponseTime()
				realtimeEvent := &dto.RealtimeEvent{}
				err = common.Unmarshal(message, realtimeEvent)
				if err != nil {
					errChan <- fmt.Errorf("error unmarshalling message: %v", err)
					return
				}

				if realtimeEvent.Type == dto.RealtimeEventTypeResponseDone {
					sawResponseDone.Store(true)
					realtimeUsage := realtimeEvent.Response.Usage
					if realtimeUsage != nil {
						usageMu.Lock()
						usage.TotalTokens += realtimeUsage.TotalTokens
						usage.InputTokens += realtimeUsage.InputTokens
						usage.OutputTokens += realtimeUsage.OutputTokens
						usage.InputTokenDetails.AudioTokens += realtimeUsage.InputTokenDetails.AudioTokens
						usage.InputTokenDetails.CachedTokens += realtimeUsage.InputTokenDetails.CachedTokens
						usage.InputTokenDetails.TextTokens += realtimeUsage.InputTokenDetails.TextTokens
						usage.OutputTokenDetails.AudioTokens += realtimeUsage.OutputTokenDetails.AudioTokens
						usage.OutputTokenDetails.TextTokens += realtimeUsage.OutputTokenDetails.TextTokens
						consumeErr := preConsumeUsage(c, info, usage, sumUsage)
						*localUsage = dto.RealtimeUsage{}
						sumUsageSnapshot := *sumUsage
						localUsageSnapshot := *localUsage
						usageMu.Unlock()
						if consumeErr != nil {
							errChan <- fmt.Errorf("error consume usage: %v", consumeErr)
							return
						}
						logger.LogInfo(c, fmt.Sprintf("realtime streaming sumUsage: %v", &sumUsageSnapshot))
						logger.LogInfo(c, fmt.Sprintf("realtime streaming localUsage: %v", &localUsageSnapshot))
					} else {
						textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
						if err != nil {
							errChan <- fmt.Errorf("error counting text token: %v", err)
							return
						}
						logger.LogInfo(c, fmt.Sprintf("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
						usageMu.Lock()
						localUsage.TotalTokens += textToken + audioToken
						info.IsFirstRequest = false
						localUsage.InputTokens += textToken + audioToken
						localUsage.InputTokenDetails.TextTokens += textToken
						localUsage.InputTokenDetails.AudioTokens += audioToken
						consumeErr := preConsumeUsage(c, info, localUsage, sumUsage)
						sumUsageSnapshot := *sumUsage
						localUsageSnapshot := *localUsage
						usageMu.Unlock()
						if consumeErr != nil {
							errChan <- fmt.Errorf("error consume usage: %v", consumeErr)
							return
						}
						logger.LogInfo(c, fmt.Sprintf("realtime streaming sumUsage: %v", &sumUsageSnapshot))
						logger.LogInfo(c, fmt.Sprintf("realtime streaming localUsage: %v", &localUsageSnapshot))
					}

				} else if realtimeEvent.Type == dto.RealtimeEventTypeSessionUpdated || realtimeEvent.Type == dto.RealtimeEventTypeSessionCreated {
					realtimeSession := realtimeEvent.Session
					if realtimeSession != nil {
						// update audio format
						info.InputAudioFormat = common.GetStringIfEmpty(realtimeSession.InputAudioFormat, info.InputAudioFormat)
						info.OutputAudioFormat = common.GetStringIfEmpty(realtimeSession.OutputAudioFormat, info.OutputAudioFormat)
					}
				} else {
					textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
					if err != nil {
						errChan <- fmt.Errorf("error counting text token: %v", err)
						return
					}
					logger.LogInfo(c, fmt.Sprintf("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
					usageMu.Lock()
					localUsage.TotalTokens += textToken + audioToken
					localUsage.OutputTokens += textToken + audioToken
					localUsage.OutputTokenDetails.TextTokens += textToken
					localUsage.OutputTokenDetails.AudioTokens += audioToken
					usageMu.Unlock()
				}

				err = helper.WssString(c, clientConn, string(message))
				if err != nil {
					errChan <- fmt.Errorf("error writing to client: %v", err)
					return
				}

				select {
				case receiveChan <- message:
				default:
				}
			}
		}
	})

	var terminalErr error
	select {
	case <-clientClosed:
	case <-targetClosed:
	case err := <-errChan:
		terminalErr = err
		logger.LogError(c, "realtime error: "+err.Error())
	case <-c.Done():
	}
	stopping.Store(true)
	_ = clientConn.Close()
	_ = targetConn.Close()
	readers.Wait()
	if terminalErr == nil && info.AttemptDispatched && !sawResponseDone.Load() {
		terminalErr = fmt.Errorf("realtime connection closed before a terminal response")
	}

	usageMu.Lock()
	if usage.TotalTokens != 0 {
		if err := preConsumeUsage(c, info, usage, sumUsage); err != nil && terminalErr == nil {
			terminalErr = fmt.Errorf("error consume final upstream usage: %w", err)
		}
	}

	if localUsage.TotalTokens != 0 {
		if err := preConsumeUsage(c, info, localUsage, sumUsage); err != nil && terminalErr == nil {
			terminalErr = fmt.Errorf("error consume final local usage: %w", err)
		}
	}
	finalUsage := *sumUsage
	usageMu.Unlock()

	if terminalErr != nil {
		return types.NewError(terminalErr, types.ErrorCodeBadResponse), &finalUsage
	}
	return nil, &finalUsage
}

func preConsumeUsage(ctx *gin.Context, info *relaycommon.RelayInfo, usage *dto.RealtimeUsage, totalUsage *dto.RealtimeUsage) error {
	if usage == nil || totalUsage == nil {
		return fmt.Errorf("invalid usage pointer")
	}

	currentUsage := *usage
	totalUsage.TotalTokens += currentUsage.TotalTokens
	totalUsage.InputTokens += currentUsage.InputTokens
	totalUsage.OutputTokens += currentUsage.OutputTokens
	totalUsage.InputTokenDetails.CachedTokens += currentUsage.InputTokenDetails.CachedTokens
	totalUsage.InputTokenDetails.TextTokens += currentUsage.InputTokenDetails.TextTokens
	totalUsage.InputTokenDetails.AudioTokens += currentUsage.InputTokenDetails.AudioTokens
	totalUsage.OutputTokenDetails.TextTokens += currentUsage.OutputTokenDetails.TextTokens
	totalUsage.OutputTokenDetails.AudioTokens += currentUsage.OutputTokenDetails.AudioTokens
	*usage = dto.RealtimeUsage{}
	return service.PreWssConsumeQuota(ctx, info, &currentUsage)
}
