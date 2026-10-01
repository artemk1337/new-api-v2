package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
)

// ErrPayMethodsNotConfigured distinguishes an initialized database with no
// persisted payment-method catalog from a database that has not created the
// options table yet. The latter may use the bootstrap snapshot; the former
// must fail closed so stale in-memory settings cannot re-enable payments.
var ErrPayMethodsNotConfigured = errors.New("payment methods are not configured")

// GetPayMethodsFromDB reads the persisted payment-method catalog. A missing
// options table is treated as an uninitialized database so callers can use
// their bootstrap snapshot; other database failures are preserved and must
// not silently re-enable stale payment methods.
func GetPayMethodsFromDB(db *gorm.DB) ([]map[string]string, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var option Option
	if err := db.Where("key = ?", "PayMethods").First(&option).Error; err != nil {
		if isMissingOptionsTableError(err) {
			return nil, gorm.ErrRecordNotFound
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPayMethodsNotConfigured
		}
		return nil, err
	}
	methods, err := operation_setting.ParsePayMethodsJSON(option.Value)
	if err != nil {
		return nil, err
	}
	return filterRetiredPayMethods(operation_setting.CanonicalizePayMethods(methods)), nil
}

func filterRetiredPayMethods(methods []map[string]string) []map[string]string {
	filtered := make([]map[string]string, 0, len(methods))
	for _, method := range methods {
		if method == nil || strings.EqualFold(strings.TrimSpace(method["type"]), PaymentMethodCreem) {
			continue
		}
		filtered = append(filtered, method)
	}
	return filtered
}

// removeRetiredCreemPayMethod removes the retired checkout from the durable
// catalog. Existing orders keep their own provider snapshot and can still be
// settled by the legacy webhook.
func removeRetiredCreemPayMethod() error {
	var option Option
	if err := DB.First(&option, "key = ?", "PayMethods").Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	methods, err := operation_setting.ParsePayMethodsJSON(option.Value)
	if err != nil {
		return err
	}
	hasCreem := false
	for _, method := range methods {
		if method != nil && strings.EqualFold(strings.TrimSpace(method["type"]), PaymentMethodCreem) {
			hasCreem = true
			break
		}
	}
	if !hasCreem {
		return nil
	}
	encoded, err := common.Marshal(filterRetiredPayMethods(operation_setting.CanonicalizePayMethods(methods)))
	if err != nil {
		return err
	}
	result := DB.Model(&Option{}).Where("key = ? AND value = ?", "PayMethods", option.Value).
		Update("value", string(encoded))
	if result.Error != nil || result.RowsAffected == 0 {
		return result.Error
	}
	return updateOptionMapFromDatabase("PayMethods", string(encoded))
}

// HasDirectUSDTMethod reports whether the catalog explicitly enables the
// direct USDT integration. Presence in persisted PayMethods, rather than the
// legacy integration flag, is the runtime activation switch.
func HasDirectUSDTMethod(methods []map[string]string) bool {
	for _, method := range methods {
		if method != nil && isDirectUSDTNetworkProvider(method["type"]) {
			return true
		}
	}
	return false
}

func isDirectUSDTNetworkProvider(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case operation_setting.DirectCryptoPaymentMethod, operation_setting.DirectUSDTTRC20PaymentMethod, operation_setting.DirectUSDTTONPaymentMethod, operation_setting.DirectUSDTSolanaPaymentMethod:
		return true
	default:
		return false
	}
}

// IsDirectUSDTMethodConfigured reads the persisted catalog. Databases created
// before the options table existed keep the old test/bootstrap behaviour and
// fall back to the legacy flag; once the table exists, a missing row or method
// is an explicit disabled state.
func IsDirectUSDTMethodConfigured() bool {
	methods, err := GetPayMethodsFromDB(DB)
	if err == nil {
		return HasDirectUSDTMethod(methods)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) || DB == nil {
		return false
	}
	var option Option
	lookupErr := DB.Where("key = ?", "PayMethods").First(&option).Error
	if isMissingOptionsTableError(lookupErr) {
		return setting.USDTTRC20Enabled
	}
	return false
}

func isMissingOptionsTableError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such table") ||
		strings.Contains(message, `relation "options" does not exist`) ||
		(strings.Contains(message, "table") && strings.Contains(message, "doesn't exist") && strings.Contains(message, "options"))
}
