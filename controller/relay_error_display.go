package controller

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
)

const (
	relayGroupUpstreamSaturatedMessage = "The upstream load for the current group is saturated. Please try again later or switch to another group."
	relayCredentialsConcurrencyMessage = "All available upstream credentials have reached their concurrency limit. Please try again later or switch to another group."
	relayNoUpstreamContentMessage      = "The upstream did not return response content. Please try again later or switch to another group."
	relayNoUpstreamTokenMessage        = "No upstream token is available. Please try again later or switch to another group."
	relayEmptyModelContentMessage      = "The upstream model returned empty content."
)

func relayDisplayErrorMessage(err *types.NewAPIError) (string, bool) {
	if err == nil {
		return "", false
	}

	raw := err.Error()
	switch err.StatusCode {
	case http.StatusTooManyRequests:
		if strings.Contains(raw, "当前分组上游负载已饱和") ||
			(strings.Contains(raw, "当前模型") && strings.Contains(raw, "上游已饱和")) {
			return relayGroupUpstreamSaturatedMessage, true
		}
		if strings.Contains(raw, "所有可用凭据均已达到并发上限") {
			return relayCredentialsConcurrencyMessage, true
		}
	case http.StatusBadRequest:
		if strings.Contains(raw, "未接收到上游响应内容") {
			return relayNoUpstreamContentMessage, true
		}
		if strings.Contains(raw, "没有可用token") {
			return relayNoUpstreamTokenMessage, true
		}
	case http.StatusInternalServerError:
		if strings.Contains(raw, "模型返回内容为空") {
			return relayEmptyModelContentMessage, true
		}
	}

	return raw, false
}

func relayErrorLogContent(err *types.NewAPIError) (content string, rawDiagnostic string, normalized bool) {
	if err == nil {
		return "", "", false
	}

	rawDiagnostic = err.MaskSensitiveErrorWithStatusCode()
	displayMessage, normalized := relayDisplayErrorMessage(err)
	if !normalized {
		return rawDiagnostic, "", false
	}
	return relayErrorWithStatusCode(err.StatusCode, displayMessage), rawDiagnostic, true
}

func prepareRelayResponseError(err *types.NewAPIError, requestId string) {
	if err == nil {
		return
	}

	displayMessage, normalized := relayDisplayErrorMessage(err)
	if !normalized {
		displayMessage = err.Error()
	}
	setRelayErrorMessage(err, common.MessageWithRequestId(displayMessage, requestId))
}

func setRelayErrorMessage(err *types.NewAPIError, message string) {
	if err == nil {
		return
	}

	err.SetMessage(message)
	switch relayErr := err.RelayError.(type) {
	case types.OpenAIError:
		relayErr.Message = message
		err.RelayError = relayErr
	case types.ClaudeError:
		relayErr.Message = message
		err.RelayError = relayErr
	}
}

func relayErrorWithStatusCode(statusCode int, message string) string {
	if statusCode == 0 {
		return message
	}
	if message == "" {
		return fmt.Sprintf("status_code=%d", statusCode)
	}
	return fmt.Sprintf("status_code=%d, %s", statusCode, message)
}
