package operation_setting

import (
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	// PaymentPendingTTLMinutes controls the fallback lifetime of an unpaid
	// top-up. Values are read from the live option map on every call.
	PaymentPendingTTLMinutes = "PaymentPendingTTLMinutes"
	// DirectCryptoPaymentMethod is the only configurable direct-crypto method.
	// The selected chain is captured in the immutable DirectCryptoPayment
	// snapshot, never encoded in a mutable PayMethods entry.
	DirectCryptoPaymentMethod = "crypto_direct"
	// The following names are historical persisted provider IDs. Keep them for
	// settlement and order history; new PayMethods must be normalized to
	// DirectCryptoPaymentMethod.
	DirectUSDTTRC20PaymentMethod  = "usdt_trc20_direct"
	DirectUSDTTONPaymentMethod    = "usdt_ton_direct"
	DirectUSDTSolanaPaymentMethod = "usdt_solana_direct"
	// PaymentCreationRateLimit and PaymentCreationRateLimitDurationMinutes are
	// deliberately options rather than environment variables so operators can
	// change abuse protection without restarting the process.
	PaymentCreationRateLimit                = "PaymentCreationRateLimit"
	PaymentCreationRateLimitDurationMinutes = "PaymentCreationRateLimitDurationMinutes"
	DefaultPaymentPendingTTL                = 24 * time.Hour
	DefaultYooKassaPendingTTL               = 15 * time.Minute
	DefaultDirectUSDTTRC20PendingTTL        = 30 * time.Minute
	// TronGrid reconciliation intentionally scans a bounded overlap.  Keep a
	// direct order's lifetime within that horizon so an active order can never
	// outlive the watcher window.
	MaxDirectUSDTTRC20PendingTTL          = 48 * time.Hour
	DefaultPaymentCreationRateLimit       = 5
	DefaultPaymentCreationRateLimitWindow = time.Minute
	// Payment creation attempts are retained only as long as the largest
	// supported active window. Keeping a year of per-user timestamps would let
	// a high-volume account grow Redis/in-memory state without bound.
	MaxPaymentCreationRateLimit              = 1000
	MaxPaymentCreationRateLimitWindowMinutes = 60
	PaymentCreationRateLimitRetention        = time.Hour
)

func optionInt(key string, fallback int) int {
	common.OptionMapRWMutex.RLock()
	value := strings.TrimSpace(common.OptionMap[key])
	common.OptionMapRWMutex.RUnlock()
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

// PendingTopUpTTL returns the configured lifetime for a payment method. A
// method may carry pending_ttl_minutes in PayMethods; the YooKassa default is
// intentionally shorter because SBP checkouts are short-lived.
func PendingTopUpTTL(paymentMethod string) time.Duration {
	method := strings.ToLower(strings.TrimSpace(paymentMethod))
	defaultTTL := DefaultPaymentPendingTTL
	if method == YooKassaSBPPaymentMethod {
		defaultTTL = DefaultYooKassaPendingTTL
	} else if method == DirectCryptoPaymentMethod || method == DirectUSDTTRC20PaymentMethod {
		defaultTTL = DefaultDirectUSDTTRC20PendingTTL
	} else if method == DirectUSDTTONPaymentMethod || method == DirectUSDTSolanaPaymentMethod {
		defaultTTL = DefaultDirectUSDTTRC20PendingTTL
	}
	for _, configured := range payMethodsSnapshot() {
		if configured == nil || !strings.EqualFold(strings.TrimSpace(configured["type"]), method) {
			continue
		}
		if minutes, err := strconv.Atoi(strings.TrimSpace(configured["pending_ttl_minutes"])); err == nil && minutes > 0 {
			if isDirectUSDTMethod(method) {
				minutes = min(minutes, int(MaxDirectUSDTTRC20PendingTTL/time.Minute))
			}
			return boundedPaymentPendingTTL(minutes)
		}
		break
	}
	if method != YooKassaSBPPaymentMethod && !isDirectUSDTMethod(method) {
		if minutes := optionInt(PaymentPendingTTLMinutes, 0); minutes > 0 {
			return boundedPaymentPendingTTL(minutes)
		}
	}
	return defaultTTL
}

func isDirectUSDTMethod(method string) bool {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case DirectCryptoPaymentMethod, DirectUSDTTRC20PaymentMethod, DirectUSDTTONPaymentMethod, DirectUSDTSolanaPaymentMethod:
		return true
	default:
		return false
	}
}

func boundedPaymentPendingTTL(minutes int) time.Duration {
	if minutes < 1 {
		return 0
	}
	if minutes > MaxPaymentPendingTTLMinutes {
		minutes = MaxPaymentPendingTTLMinutes
	}
	return time.Duration(minutes) * time.Minute
}

func PaymentCreationLimit() (int, time.Duration) {
	count := optionInt(PaymentCreationRateLimit, DefaultPaymentCreationRateLimit)
	if count > MaxPaymentCreationRateLimit {
		count = MaxPaymentCreationRateLimit
	}
	minutes := optionInt(PaymentCreationRateLimitDurationMinutes, 1)
	if minutes > MaxPaymentCreationRateLimitWindowMinutes {
		minutes = MaxPaymentCreationRateLimitWindowMinutes
	}
	return count, time.Duration(minutes) * time.Minute
}
