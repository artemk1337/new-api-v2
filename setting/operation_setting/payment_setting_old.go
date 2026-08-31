/**
此文件为旧版支付设置文件，如需增加新的参数、变量等，请在 payment_setting.go 中添加
This file is the old version of the payment settings file. If you need to add new parameters, variables, etc., please add them in payment_setting.go
*/

package operation_setting

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

var PayAddress = ""
var CustomCallbackAddress = ""
var EpayId = ""
var EpayKey = ""
var Price = 7.3
var MinTopUp = 1.0
var USDExchangeRate = 7.3

var PayMethods = []map[string]string{
	{
		"name": "支付宝",
		"icon": "SiAlipay",
		"type": "alipay",
	},
	{
		"name": "微信",
		"icon": "SiWechat",
		"type": "wxpay",
	},
	{
		"name":      "自定义1",
		"icon":      "LuCreditCard",
		"type":      "custom1",
		"min_topup": "50",
	},
}

var payMethodsMutex sync.RWMutex

const MaxPaymentPendingTTLMinutes = 525600

const maxPendingTopUpTTLMinutes = MaxPaymentPendingTTLMinutes

// PayMethodsSnapshot returns a deep, read-only copy that callers may safely
// inspect without racing an option update.
func PayMethodsSnapshot() []map[string]string {
	payMethodsMutex.RLock()
	defer payMethodsMutex.RUnlock()
	snapshot := make([]map[string]string, len(PayMethods))
	for i, method := range PayMethods {
		if method == nil {
			continue
		}
		snapshot[i] = make(map[string]string, len(method))
		for key, value := range method {
			snapshot[i][key] = value
		}
	}
	return snapshot
}

func payMethodsSnapshot() []map[string]string { return PayMethodsSnapshot() }

const YooKassaSBPPaymentMethod = "yookassa_sbp"

// DirectUSDTTRC20PayMethodsMigratedOption prevents the legacy enable flag from
// re-adding a method after an operator deliberately removes it from the
// persisted catalog. The flag is an internal migration marker; PayMethods is
// the runtime source of truth.
const DirectUSDTTRC20PayMethodsMigratedOption = "USDTTRC20PayMethodsMigrated"

func UpdatePayMethodsByJsonString(jsonString string) error {
	methods, err := ParsePayMethodsJSON(jsonString)
	if err != nil {
		return err
	}
	if err := ValidatePayMethods(methods); err != nil {
		return err
	}
	methods = CanonicalizePayMethods(methods)
	payMethodsMutex.Lock()
	PayMethods = methods
	payMethodsMutex.Unlock()
	return nil
}

// ParsePayMethodsJSON accepts the historical string-valued catalog and the
// frontend's typed boolean admin_only field, converting both to the stable
// backend representation. Unknown non-scalar values are rejected.
func ParsePayMethodsJSON(jsonString string) ([]map[string]string, error) {
	var raw []map[string]interface{}
	if err := common.Unmarshal([]byte(jsonString), &raw); err != nil {
		return nil, err
	}
	methods := make([]map[string]string, len(raw))
	for i, item := range raw {
		if item == nil {
			continue
		}
		method := make(map[string]string, len(item))
		for key, value := range item {
			switch typed := value.(type) {
			case string:
				method[key] = typed
			case bool:
				method[key] = strconv.FormatBool(typed)
			case float64:
				method[key] = strconv.FormatFloat(typed, 'f', -1, 64)
			default:
				return nil, fmt.Errorf("PayMethods field %q must be a scalar", key)
			}
		}
		methods[i] = method
	}
	return methods, nil
}

// ValidatePayMethods validates operator-controlled method metadata before it
// reaches the live payment configuration. An omitted or blank TTL inherits the
// gateway/default policy; an explicit TTL must be bounded to one year.
func ValidatePayMethods(methods []map[string]string) error {
	for _, method := range methods {
		if method == nil {
			continue
		}
		ttl := strings.TrimSpace(method["pending_ttl_minutes"])
		if ttl != "" && !strings.EqualFold(ttl, "inherit") {
			minutes, err := strconv.Atoi(ttl)
			maxMinutes := maxPendingTopUpTTLMinutes
			if isDirectUSDTNetworkMethod(method["type"]) {
				maxMinutes = int(MaxDirectUSDTTRC20PendingTTL / time.Minute)
			}
			if err != nil || minutes < 1 || minutes > maxMinutes {
				return fmt.Errorf("pending_ttl_minutes must be an integer between 1 and %d", maxMinutes)
			}
		}
		minimum := strings.TrimSpace(method["min_topup"])
		if minimum != "" {
			value, parseErr := strconv.ParseFloat(minimum, 64)
			if parseErr != nil || value <= 0 || value > 1e12 || math.IsNaN(value) || math.IsInf(value, 0) {
				return fmt.Errorf("min_topup must be a positive finite number")
			}
		}
		adminOnly := strings.TrimSpace(method["admin_only"])
		if adminOnly != "" && !strings.EqualFold(adminOnly, "true") && !strings.EqualFold(adminOnly, "false") {
			return fmt.Errorf("admin_only must be true or false")
		}
		// Older catalogs did not have a coefficient group, so an omitted value
		// keeps the historical user-group fallback. A newly explicit group must
		// exist; silently treating a typo as coefficient 1 changes billing.
		if group := strings.TrimSpace(method["topup_group"]); group != "" && !common.HasTopupGroupRatio(group) {
			return fmt.Errorf("topup_group %q is not configured", group)
		}
	}
	return nil
}

// ValidatePayMethodsForSave protects the transition from old catalogs that
// predate topup_group to newly edited catalogs. `type` is the stable payment
// method identity. Direct-USDT network aliases are canonicalized to the one
// Crypto parent before comparison.
//
// Existing methods which already omitted the group stay compatible. Every new
// method must name an existing coefficient group, and a method that already
// had a group cannot be changed to omit it. Candidate duplicate identities are
// rejected: the payment runtime resolves by type, so accepting duplicates
// would make old-vs-new ownership of the group ambiguous.
func ValidatePayMethodsForSave(methods, persisted []map[string]string) error {
	if err := ValidatePayMethods(methods); err != nil {
		return err
	}
	candidate := CanonicalizePayMethods(methods)
	existing := CanonicalizePayMethods(persisted)
	existingByType := make(map[string]map[string]string, len(existing))
	for _, method := range existing {
		if method == nil {
			continue
		}
		paymentType := strings.ToLower(strings.TrimSpace(method["type"]))
		if paymentType != "" {
			existingByType[paymentType] = method
		}
	}
	seen := make(map[string]struct{}, len(candidate))
	for _, method := range candidate {
		if method == nil {
			continue
		}
		paymentType := strings.ToLower(strings.TrimSpace(method["type"]))
		if paymentType == "" {
			return fmt.Errorf("payment method type is required")
		}
		if _, duplicate := seen[paymentType]; duplicate {
			return fmt.Errorf("payment method type %q must be unique", paymentType)
		}
		seen[paymentType] = struct{}{}
		if strings.TrimSpace(method["topup_group"]) != "" {
			continue
		}
		previous, exists := existingByType[paymentType]
		if !exists || strings.TrimSpace(previous["topup_group"]) != "" {
			return fmt.Errorf("topup_group is required for payment method %q", paymentType)
		}
	}
	return nil
}

// IsPayMethodAdminOnly safely interprets the persisted flag. Missing and
// malformed values remain public for backwards compatibility; malformed
// values are rejected on persistence by ValidatePayMethods.
func IsPayMethodAdminOnly(method map[string]string) bool {
	return method != nil && strings.EqualFold(strings.TrimSpace(method["admin_only"]), "true")
}

func FilterPayMethodsForRole(methods []map[string]string, isAdmin bool) []map[string]string {
	if isAdmin {
		return methods
	}
	filtered := make([]map[string]string, 0, len(methods))
	for _, method := range methods {
		if !IsPayMethodAdminOnly(method) {
			filtered = append(filtered, method)
		}
	}
	return filtered
}

// NormalizePayMethods keeps legacy configurations valid while adding the
// optional provider metadata used by the wallet. Legacy currency fields are
// removed because settlement currency is owned by each provider integration.
func NormalizePayMethods(methods []map[string]string) {
	for _, method := range methods {
		if method == nil {
			continue
		}
		paymentType := strings.TrimSpace(method["type"])
		if strings.EqualFold(strings.TrimSpace(method["pending_ttl_minutes"]), "inherit") || strings.TrimSpace(method["pending_ttl_minutes"]) == "" {
			delete(method, "pending_ttl_minutes")
		}
		if adminOnly := strings.TrimSpace(method["admin_only"]); adminOnly != "" {
			if strings.EqualFold(adminOnly, "true") || strings.EqualFold(adminOnly, "false") {
				method["admin_only"] = strings.ToLower(adminOnly)
			}
		}
		canonicalType := strings.ToLower(paymentType)
		switch canonicalType {
		case "nowpayments", "yookassa_sbp", "stripe", "waffo", "waffo_pancake":
			method["type"] = canonicalType
		case DirectCryptoPaymentMethod, DirectUSDTTRC20PaymentMethod, DirectUSDTTONPaymentMethod, DirectUSDTSolanaPaymentMethod:
			method["type"] = DirectCryptoPaymentMethod
			method["name"] = "Crypto"
		}
		if canonicalType == YooKassaSBPPaymentMethod {
			switch strings.ToLower(strings.TrimSpace(method["name"])) {
			case "сбп / yookassa", "yookassa sbp":
				method["name"] = "СБП"
			}
		}
		// Settlement currency belongs to the provider integration settings, not
		// PayMethods metadata. Drop legacy values while retaining all other keys.
		delete(method, "currency")
	}
}

func isDirectUSDTNetworkMethod(method string) bool {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case DirectCryptoPaymentMethod, DirectUSDTTRC20PaymentMethod, DirectUSDTTONPaymentMethod, DirectUSDTSolanaPaymentMethod:
		return true
	default:
		return false
	}
}

// CanonicalizePayMethods normalizes persisted metadata and reduces every
// legacy network-specific direct entry to one Crypto parent. An explicit
// crypto_direct entry wins its metadata; otherwise the first legacy entry in
// catalog order wins. The parent occupies the first direct entry's position,
// keeping operator ordering stable while preventing network-specific policy
// bypasses.
func CanonicalizePayMethods(methods []map[string]string) []map[string]string {
	var explicitDirect map[string]string
	for _, method := range methods {
		if method != nil && strings.EqualFold(strings.TrimSpace(method["type"]), DirectCryptoPaymentMethod) {
			explicitDirect = method
			break
		}
	}
	result := make([]map[string]string, 0, len(methods))
	hasDirect := false
	for _, method := range methods {
		if method != nil {
			copyMethod := make(map[string]string, len(method))
			for key, value := range method {
				copyMethod[key] = value
			}
			method = copyMethod
		}
		if method != nil && isDirectUSDTNetworkMethod(method["type"]) {
			if hasDirect {
				continue
			}
			hasDirect = true
			if explicitDirect != nil {
				method = make(map[string]string, len(explicitDirect))
				for key, value := range explicitDirect {
					method[key] = value
				}
			}
		}
		result = append(result, method)
	}
	NormalizePayMethods(result)
	return result
}

// EnsureYooKassaPayMethod returns a normalized copy of PayMethods and adds the
// editable SBP method when the YooKassa integration is active. Existing
// methods are retained as-is apart from missing common fields, so an operator
// can keep custom labels, icons, and pricing groups across the migration.
func EnsureYooKassaPayMethod(methods []map[string]string, enabled bool) ([]map[string]string, bool) {
	normalized := make([]map[string]string, 0, len(methods)+1)
	changed := false
	hasYooKassa := false
	for _, method := range methods {
		if method == nil {
			normalized = append(normalized, nil)
			continue
		}
		copyMethod := make(map[string]string, len(method)+2)
		for key, value := range method {
			copyMethod[key] = value
		}
		if strings.EqualFold(strings.TrimSpace(copyMethod["type"]), YooKassaSBPPaymentMethod) {
			if hasYooKassa {
				changed = true
				continue
			}
			hasYooKassa = true
			if copyMethod["type"] != YooKassaSBPPaymentMethod {
				copyMethod["type"] = YooKassaSBPPaymentMethod
				changed = true
			}
			if _, exists := copyMethod["currency"]; exists {
				delete(copyMethod, "currency")
				changed = true
			}
			if strings.TrimSpace(copyMethod["topup_group"]) == "" {
				copyMethod["topup_group"] = "default"
				changed = true
			}
		}
		normalized = append(normalized, copyMethod)
	}
	if enabled && !hasYooKassa {
		normalized = append(normalized, map[string]string{
			"name":        "СБП",
			"type":        YooKassaSBPPaymentMethod,
			"topup_group": "default",
		})
		changed = true
	}
	return normalized, changed
}

func PayMethods2JsonString() string {
	jsonBytes, err := common.Marshal(PayMethodsSnapshot())
	if err != nil {
		return "[]"
	}
	return string(jsonBytes)
}

func ContainsPayMethod(method string) bool {
	for _, payMethod := range PayMethodsSnapshot() {
		if payMethod != nil && strings.EqualFold(strings.TrimSpace(payMethod["type"]), strings.TrimSpace(method)) {
			return true
		}
	}
	return false
}
