package billing_setting

import (
	"fmt"
	"math"
	"sync"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/samber/lo"
)

const (
	BillingModeRatio       = "ratio"
	BillingModeTieredExpr  = "tiered_expr"
	BillingModeField       = "billing_mode"
	BillingExprField       = "billing_expr"
	TaskPriceUnitPerCall   = "per_call"
	TaskPriceUnitPerSecond = "per_second"
	TaskPriceUnitField     = "task_price_unit"
)

// BillingSetting is managed by config.GlobalConfig.Register.
// DB keys: billing_setting.billing_mode, billing_setting.billing_expr,
// billing_setting.task_price_unit.
type BillingSetting struct {
	BillingMode   map[string]string `json:"billing_mode"`
	BillingExpr   map[string]string `json:"billing_expr"`
	TaskPriceUnit map[string]string `json:"task_price_unit"`
}

var billingSetting = BillingSetting{
	BillingMode:   make(map[string]string),
	BillingExpr:   make(map[string]string),
	TaskPriceUnit: make(map[string]string),
}
var billingSettingMutex sync.RWMutex

func init() {
	config.GlobalConfig.Register("billing_setting", &billingSetting)
}

// ---------------------------------------------------------------------------
// Read accessors (hot path, must be fast)
// ---------------------------------------------------------------------------

func GetBillingMode(model string) string {
	billingSettingMutex.RLock()
	defer billingSettingMutex.RUnlock()
	if mode, ok := billingSetting.BillingMode[model]; ok {
		return mode
	}
	return BillingModeRatio
}

func GetBillingExpr(model string) (string, bool) {
	billingSettingMutex.RLock()
	defer billingSettingMutex.RUnlock()
	expr, ok := billingSetting.BillingExpr[model]
	return expr, ok
}

func GetTaskPriceUnit(model string) string {
	billingSettingMutex.RLock()
	defer billingSettingMutex.RUnlock()
	if unit, ok := billingSetting.TaskPriceUnit[model]; ok {
		return unit
	}
	return TaskPriceUnitPerSecond
}

func IsTaskPriceUnit(unit string) bool {
	return unit == TaskPriceUnitPerCall || unit == TaskPriceUnitPerSecond
}

func GetBillingModeCopy() map[string]string {
	billingSettingMutex.RLock()
	defer billingSettingMutex.RUnlock()
	return lo.Assign(billingSetting.BillingMode)
}

func GetBillingExprCopy() map[string]string {
	billingSettingMutex.RLock()
	defer billingSettingMutex.RUnlock()
	return lo.Assign(billingSetting.BillingExpr)
}

func GetTaskPriceUnitCopy() map[string]string {
	billingSettingMutex.RLock()
	defer billingSettingMutex.RUnlock()
	return lo.Assign(billingSetting.TaskPriceUnit)
}

// UpdateFromMap replaces billing maps while keeping all read accessors safe.
func UpdateFromMap(values map[string]string) error {
	billingSettingMutex.Lock()
	defer billingSettingMutex.Unlock()
	return config.UpdateConfigFromMap(&billingSetting, values)
}

func GetPricingSyncData(base map[string]any) map[string]any {
	billingSettingMutex.RLock()
	defer billingSettingMutex.RUnlock()
	extra := make(map[string]any, 3)
	if modes := lo.Assign(billingSetting.BillingMode); len(modes) > 0 {
		extra[BillingModeField] = modes
	}
	if exprs := lo.Assign(billingSetting.BillingExpr); len(exprs) > 0 {
		extra[BillingExprField] = exprs
	}
	if units := lo.Assign(billingSetting.TaskPriceUnit); len(units) > 0 {
		extra[TaskPriceUnitField] = units
	}
	return lo.Assign(base, extra)
}

// ---------------------------------------------------------------------------
// Smoke test (called externally for validation before save)
// ---------------------------------------------------------------------------

func SmokeTestExpr(exprStr string) error {
	return smokeTestExpr(exprStr)
}

func smokeTestExpr(exprStr string) error {
	if !billingexpr.ReasoningSplitIsSafe(exprStr) {
		return fmt.Errorf("reasoning pricing requires linear output terms and total-output conditions")
	}
	vectors := []billingexpr.TokenParams{
		{P: 0, C: 0, Len: 0},
		{P: 1000, C: 1000, Len: 1000},
		{P: 100000, C: 100000, Len: 100000},
		{P: 1000000, C: 1000000, Len: 1000000},
		{CR: 1000, Len: 1000},
		{CC: 1000, Len: 1000},
		{CC1h: 1000, Len: 1000},
		{Img: 1000, Len: 1000},
		{ImgO: 1000, Len: 1000},
		{AI: 1000, Len: 1000},
		{AO: 1000, Len: 1000},
		{RT: 1000, Len: 1000},
		{C: 1000, RT: 1000, Len: 1000},
	}
	requests := []billingexpr.RequestInput{
		{},
		{
			Headers: map[string]string{
				"anthropic-beta": "fast-mode-2026-02-01",
			},
			Body: []byte(`{"service_tier":"fast","stream_options":{"include_usage":true},"messages":[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21]}`),
		},
	}

	for _, v := range vectors {
		for _, request := range requests {
			result, _, err := billingexpr.RunExprWithRequest(exprStr, v, request)
			if err != nil {
				return fmt.Errorf("vector {p=%g, c=%g}: run failed: %w", v.P, v.C, err)
			}
			if math.IsNaN(result) || math.IsInf(result, 0) {
				return fmt.Errorf("vector {p=%g, c=%g}: result must be finite", v.P, v.C)
			}
			if result < 0 {
				return fmt.Errorf("vector {p=%g, c=%g}: result %f < 0", v.P, v.C, result)
			}
		}
	}
	return nil
}
