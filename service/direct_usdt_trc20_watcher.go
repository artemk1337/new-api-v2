package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

const (
	directUSDTWatcherInterval   = 30 * time.Second
	directUSDTWatcherMaxBackoff = 5 * time.Minute
	directUSDTReconcileOverlap  = operation_setting.MaxDirectUSDTTRC20PendingTTL
	directUSDTPageLimit         = 200
	directUSDTMaxPages          = 100
)

var (
	DirectUSDTTRC20HTTPClient = &http.Client{Timeout: 15 * time.Second}
	// Tests may point this variable at an httptest server. Production code
	// keeps the endpoint immutable through the setting package constant.
	DirectUSDTTRC20TronGridBaseURL = setting.USDTTRC20TronGridAPIBaseURL
	directUSDTWatcherMu            sync.Mutex
	directUSDTWatcherCancel        context.CancelFunc
)

type tronGridTransfer struct {
	TransactionID  string `json:"transaction_id"`
	From           string `json:"from"`
	To             string `json:"to"`
	Value          string `json:"value"`
	EventIndex     any    `json:"event_index"`
	BlockTimestamp int64  `json:"block_timestamp"`
	BlockNumber    int64  `json:"block_number"`
	TokenInfo      struct {
		Address string `json:"address"`
	} `json:"token_info"`
	TransactionRet []struct {
		ContractRet string `json:"contractRet"`
	} `json:"transaction_ret"`
}

type tronGridTransfersResponse struct {
	Data    []tronGridTransfer `json:"data"`
	Success *bool              `json:"success"`
	Meta    struct {
		Fingerprint string `json:"fingerprint"`
	} `json:"meta"`
}

// StartDirectUSDTTRC20Watcher starts a cancellable reconciliation worker. It
// is safe to call more than once; every process may run a worker because the
// database settlement path is idempotent and serializes quota crediting.
func StartDirectUSDTTRC20Watcher() func() {
	directUSDTWatcherMu.Lock()
	if directUSDTWatcherCancel != nil {
		cancel := directUSDTWatcherCancel
		directUSDTWatcherMu.Unlock()
		return cancel
	}
	ctx, cancel := context.WithCancel(context.Background())
	directUSDTWatcherCancel = cancel
	directUSDTWatcherMu.Unlock()
	go directUSDTWatcherLoop(ctx)
	return cancel
}

func StopDirectUSDTTRC20Watcher() {
	directUSDTWatcherMu.Lock()
	cancel := directUSDTWatcherCancel
	directUSDTWatcherCancel = nil
	directUSDTWatcherMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func directUSDTWatcherLoop(ctx context.Context) {
	backoff := time.Duration(0)
	for {
		if backoff > 0 {
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return
			case <-timer.C:
			}
		}
		if err := PollDirectUSDTTRC20Once(ctx); err != nil {
			common.SysLog("direct USDT TRC20 watcher failed: " + err.Error())
			if backoff == 0 {
				backoff = 5 * time.Second
			} else if backoff < directUSDTWatcherMaxBackoff {
				backoff *= 2
				if backoff > directUSDTWatcherMaxBackoff {
					backoff = directUSDTWatcherMaxBackoff
				}
			}
			continue
		}
		backoff = 0
		timer := time.NewTimer(directUSDTWatcherInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}

// PollDirectUSDTTRC20Once performs one safe-overlap reconciliation pass. It
// deliberately has no mutable cursor: every pass asks TronGrid for the last
// 48 hours and relies on event-idempotent settlement, so process crashes and
// pagination restarts cannot lose a payment.
func PollDirectUSDTTRC20Once(ctx context.Context) error {
	now := time.Now()
	addresses := make([]string, 0, 1)
	// The enable flag and current configuration gate only new order creation and
	// publication. Reconciliation must continue for immutable snapshots after
	// an operator disables the method or rotates/temporarily breaks settings.
	if currentAddress := strings.TrimSpace(setting.USDTTRC20ReceivingAddress); setting.ValidateTRONAddress(currentAddress) == nil {
		addresses = append(addresses, currentAddress)
	}
	// Include every address captured by an existing order. This is deliberately
	// additive to the current setting so rotating the receiving wallet does not
	// strand orders created with the previous address.
	if model.DB == nil {
		return nil
	}
	activeAddresses, addressErr := model.GetActivePendingDirectUSDTPaymentAddresses(now.Unix())
	if addressErr != nil {
		return addressErr
	}
	seenAddresses := make(map[string]struct{}, len(addresses)+len(activeAddresses))
	for _, address := range append(addresses, activeAddresses...) {
		address = strings.TrimSpace(address)
		if address == "" {
			continue
		}
		if err := setting.ValidateTRONAddress(address); err != nil {
			// A corrupt historical row must not turn into an arbitrary TronGrid
			// request.  Leave that order for manual recovery and continue with
			// every other validated receiving address.
			continue
		}
		if _, seen := seenAddresses[address]; seen {
			continue
		}
		seenAddresses[address] = struct{}{}

		transfers, err := fetchDirectUSDTTransfers(ctx, address, strings.TrimSpace(setting.USDTTRC20APIKey), now)
		if err != nil {
			return err
		}
		for i := range transfers {
			transfer := transfers[i]
			if !validDirectUSDTTransfer(transfer, address) ||
				transfer.BlockTimestamp < now.Add(-directUSDTReconcileOverlap).UnixMilli() {
				// TronGrid should honor min_timestamp, but keep the horizon check
				// local as well so an ignored/forged provider parameter cannot make
				// a stale event eligible for settlement.
				continue
			}
			amountUnits, parseErr := strconv.ParseUint(strings.TrimSpace(transfer.Value), 10, 64)
			if parseErr != nil || amountUnits == 0 {
				continue
			}
			eventIndex := directUSDTTransferEventIndex(transfer)
			eventID := model.DirectUSDTEventID(transfer.TransactionID, eventIndex)
			payments, lookupErr := model.GetPendingDirectUSDTPayments(amountUnits, address)
			if lookupErr != nil {
				return lookupErr
			}
			for _, payment := range payments {
				event := model.DirectUSDTTransferEvent{
					TradeNo: payment.TradeNo, TxHash: transfer.TransactionID,
					EventIndex: eventIndex, EventID: eventID,
					Contract: transfer.TokenInfo.Address, To: transfer.To,
					AmountUnits:   amountUnits,
					Confirmations: uint64(setting.USDTTRC20MinConfirmations), Confirmed: true,
					BlockTimestamp: transfer.BlockTimestamp,
				}
				if settleErr := model.SettleDirectUSDTTRC20Event(event); settleErr != nil && !isPermanentDirectSettlementError(settleErr) {
					return settleErr
				}
			}
		}
	}
	return nil
}

// A chain event can be permanently unsuitable for one local order (for
// example an old timestamp or an amount collision).  It must not prevent
// later valid transfers in the same page from being reconciled.  Database,
// network and other unknown failures remain fail-closed and abort the pass.
func isPermanentDirectSettlementError(err error) bool {
	return errors.Is(err, model.ErrDirectPaymentInvalid) ||
		errors.Is(err, model.ErrDirectPaymentExpired) ||
		errors.Is(err, model.ErrDirectPaymentAlreadySettled) ||
		errors.Is(err, model.ErrDirectPaymentEventAlreadyUsed) ||
		errors.Is(err, model.ErrDirectPaymentAmountMismatch) ||
		errors.Is(err, model.ErrTopUpStatusInvalid) ||
		errors.Is(err, model.ErrTopUpNotFound) ||
		errors.Is(err, model.ErrPaymentMethodMismatch) ||
		errors.Is(err, model.ErrTopUpUserNotFound)
}

func fetchDirectUSDTTransfers(ctx context.Context, address, apiKey string, now time.Time) ([]tronGridTransfer, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(DirectUSDTTRC20TronGridBaseURL), "/")
	if baseURL != setting.USDTTRC20TronGridAPIBaseURL {
		// The override exists solely for tests. Production must not be pointed at
		// an arbitrary endpoint through runtime configuration.
		if strings.HasPrefix(baseURL, "http://") || strings.HasPrefix(baseURL, "https://") {
			// Allow an explicit test server URL; operators cannot configure it via
			// the public settings API.
		} else {
			return nil, fmt.Errorf("invalid TronGrid endpoint")
		}
	}
	var transfers []tronGridTransfer
	fingerprint := ""
	minTimestamp := now.Add(-directUSDTReconcileOverlap).UnixMilli()
	completed := false
	for page := 0; page < directUSDTMaxPages; page++ {
		endpoint, err := url.Parse(baseURL + "/v1/accounts/" + url.PathEscape(address) + "/transactions/trc20")
		if err != nil {
			return nil, err
		}
		query := endpoint.Query()
		query.Set("limit", strconv.Itoa(directUSDTPageLimit))
		query.Set("only_confirmed", "true")
		query.Set("only_to", "true")
		query.Set("contract_address", setting.USDTTRC20Contract)
		query.Set("min_timestamp", strconv.FormatInt(minTimestamp, 10))
		if fingerprint != "" {
			query.Set("fingerprint", fingerprint)
		}
		endpoint.RawQuery = query.Encode()
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Accept", "application/json")
		request.Header.Set("User-Agent", "new-api-usdt-trc20-watcher")
		if strings.TrimSpace(apiKey) != "" {
			request.Header.Set("TRON-PRO-API-KEY", strings.TrimSpace(apiKey))
		}
		response, err := DirectUSDTTRC20HTTPClient.Do(request)
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 8<<20))
		closeErr := response.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return nil, fmt.Errorf("TronGrid returned HTTP %d", response.StatusCode)
		}
		var payload tronGridTransfersResponse
		if err := common.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("invalid TronGrid response: %w", err)
		}
		if payload.Success != nil && !*payload.Success {
			return nil, fmt.Errorf("TronGrid response was not successful")
		}
		transfers = append(transfers, payload.Data...)
		fingerprint = strings.TrimSpace(payload.Meta.Fingerprint)
		if fingerprint == "" || len(payload.Data) == 0 {
			completed = true
			break
		}
	}
	if !completed && fingerprint != "" {
		return nil, fmt.Errorf("TronGrid pagination limit reached with fingerprint remaining")
	}
	return transfers, nil
}

func validDirectUSDTTransfer(transfer tronGridTransfer, receivingAddress string) bool {
	if strings.TrimSpace(transfer.TransactionID) == "" ||
		strings.TrimSpace(transfer.To) != strings.TrimSpace(receivingAddress) ||
		strings.TrimSpace(transfer.TokenInfo.Address) != setting.USDTTRC20Contract {
		return false
	}
	for _, result := range transfer.TransactionRet {
		if result.ContractRet != "" && !strings.EqualFold(result.ContractRet, "SUCCESS") {
			return false
		}
	}
	return true
}

func directUSDTTransferEventIndex(transfer tronGridTransfer) string {
	if value := strings.TrimSpace(fmt.Sprint(transfer.EventIndex)); value != "" && value != "<nil>" {
		return value
	}
	// Older TronGrid responses omit event_index. A deterministic transfer
	// digest keeps multiple Transfer logs in one transaction distinct while
	// remaining stable across overlapping reconciliation passes.
	digest := sha256.Sum256([]byte(strings.Join([]string{
		transfer.From, transfer.To, transfer.Value,
		strconv.FormatInt(transfer.BlockTimestamp, 10), strconv.FormatInt(transfer.BlockNumber, 10),
	}, "|")))
	return "h" + fmt.Sprintf("%x", digest[:12])
}
