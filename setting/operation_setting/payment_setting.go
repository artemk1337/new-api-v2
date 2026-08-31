package operation_setting

import (
	"errors"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/shopspring/decimal"
)

type CashbackThreshold struct {
	MinAmount       float64 `json:"min_amount"`
	CashbackPercent float64 `json:"cashback_percent"`
}

type AmountCashbackConfig []CashbackThreshold

func (cashbacks AmountCashbackConfig) MarshalJSON() ([]byte, error) {
	if cashbacks == nil {
		return common.Marshal([]CashbackThreshold{})
	}
	return common.Marshal([]CashbackThreshold(cashbacks))
}

func (cashbacks *AmountCashbackConfig) UnmarshalJSON(data []byte) error {
	jsonType := common.GetJsonType(data)
	if jsonType == "null" {
		*cashbacks = AmountCashbackConfig{}
		return nil
	}
	if jsonType != "array" {
		return errors.New("amount_cashback must be a JSON array")
	}

	var payload []struct {
		MinAmount       *float64 `json:"min_amount"`
		CashbackPercent *float64 `json:"cashback_percent"`
	}
	if err := common.Unmarshal(data, &payload); err != nil {
		return err
	}

	decoded := make(AmountCashbackConfig, len(payload))
	for i, item := range payload {
		if item.MinAmount == nil {
			return errors.New("cashback min_amount must be present and be a JSON number")
		}
		if item.CashbackPercent == nil {
			return errors.New("cashback_percent must be present and be a JSON number")
		}
		decoded[i] = CashbackThreshold{
			MinAmount:       *item.MinAmount,
			CashbackPercent: *item.CashbackPercent,
		}
	}
	if err := ValidateAmountCashback(decoded); err != nil {
		return err
	}
	*cashbacks = decoded
	return nil
}

func ValidateAmountCashback(cashbacks AmountCashbackConfig) error {
	amounts := make(map[float64]struct{}, len(cashbacks))
	for _, cashback := range cashbacks {
		if math.IsNaN(cashback.MinAmount) || math.IsInf(cashback.MinAmount, 0) {
			return errors.New("cashback min_amount must be finite")
		}
		if cashback.MinAmount < 0 {
			return errors.New("cashback min_amount must be greater than or equal to zero")
		}
		if _, exists := amounts[cashback.MinAmount]; exists {
			return errors.New("cashback min_amount values must be unique")
		}
		amounts[cashback.MinAmount] = struct{}{}

		if math.IsNaN(cashback.CashbackPercent) || math.IsInf(cashback.CashbackPercent, 0) {
			return errors.New("cashback_percent must be finite")
		}
		if cashback.CashbackPercent < 0 || cashback.CashbackPercent > 100 {
			return errors.New("cashback_percent must be between 0 and 100")
		}
	}
	return nil
}

func (cashbacks AmountCashbackConfig) CashbackPercentForAmount(amount float64) float64 {
	amountDecimal := decimal.NewFromFloat(amount)
	bestMinAmount := decimal.Zero
	percent := decimal.Zero
	maxPercent := decimal.NewFromInt(100)
	found := false

	for _, cashback := range cashbacks {
		minAmount := decimal.NewFromFloat(cashback.MinAmount)
		cashbackPercent := decimal.NewFromFloat(cashback.CashbackPercent)
		if cashbackPercent.IsNegative() || cashbackPercent.GreaterThan(maxPercent) || minAmount.IsNegative() || minAmount.GreaterThan(amountDecimal) || (found && !minAmount.GreaterThan(bestMinAmount)) {
			continue
		}
		bestMinAmount = minAmount
		percent = cashbackPercent
		found = true
	}

	return percent.InexactFloat64()
}

type PaymentSetting struct {
	AmountOptions         []float64            `json:"amount_options"`
	AmountCashback        AmountCashbackConfig `json:"amount_cashback"`
	ManualTopupEnabled    bool                 `json:"manual_topup_enabled"`
	ManualTopupMinAmount  float64              `json:"manual_topup_min_amount"`
	ManualTopupContactURL string               `json:"manual_topup_contact_url"`

	ComplianceConfirmed    bool   `json:"compliance_confirmed"`
	ComplianceTermsVersion string `json:"compliance_terms_version"`
	ComplianceConfirmedAt  int64  `json:"compliance_confirmed_at"`
	ComplianceConfirmedBy  int    `json:"compliance_confirmed_by"`
	ComplianceConfirmedIP  string `json:"compliance_confirmed_ip"`
}

const CurrentComplianceTermsVersion = "v1"

// 默认配置
var paymentSetting = PaymentSetting{
	AmountOptions:        []float64{10, 20, 50, 100, 200, 500},
	ManualTopupMinAmount: 5000,
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("payment_setting", &paymentSetting)
}

func GetPaymentSetting() *PaymentSetting {
	return &paymentSetting
}

func IsPaymentComplianceConfirmed() bool {
	return true
}
