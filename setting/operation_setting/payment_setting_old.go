/**
此文件为旧版支付设置文件，如需增加新的参数、变量等，请在 payment_setting.go 中添加
This file is the old version of the payment settings file. If you need to add new parameters, variables, etc., please add them in payment_setting.go
*/

package operation_setting

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

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

func UpdatePayMethodsByJsonString(jsonString string) error {
	var methods []map[string]string
	if err := common.Unmarshal([]byte(jsonString), &methods); err != nil {
		return err
	}
	if err := ValidatePayMethods(methods); err != nil {
		return err
	}
	NormalizePayMethods(methods)
	payMethodsMutex.Lock()
	PayMethods = methods
	payMethodsMutex.Unlock()
	return nil
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
		if ttl == "" || strings.EqualFold(ttl, "inherit") {
			continue
		}
		minutes, err := strconv.Atoi(ttl)
		if err != nil || minutes < 1 || minutes > maxPendingTopUpTTLMinutes {
			return fmt.Errorf("pending_ttl_minutes must be an integer between 1 and %d", maxPendingTopUpTTLMinutes)
		}
	}
	return nil
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
		canonicalType := strings.ToLower(paymentType)
		switch canonicalType {
		case "nowpayments", "yookassa_sbp", "stripe", "waffo", "waffo_pancake":
			method["type"] = canonicalType
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
		// Built-in gateways read their minimum from the provider integration
		// settings (for example StripeMinTopUp or WaffoMinTopUp). Keeping a
		// second PayMethods minimum creates a misleading, unenforced setting.
		switch canonicalType {
		case "stripe", "waffo", "waffo_pancake", "yookassa_sbp", "nowpayments":
			delete(method, "min_topup")
		}
	}
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
		if payMethod["type"] == method {
			return true
		}
	}
	return false
}
