package controller

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const directUSDTMethod = model.DirectUSDTTRC20Provider

const directUSDTBaseUnits = model.DirectUSDTMinBaseUnits

type DirectUSDTTopUpRequest struct {
	Amount        decimal.Decimal `json:"amount"`
	PaymentMethod string          `json:"payment_method"`
}

func directUSDTAmountUnits(amount decimal.Decimal) (uint64, error) {
	if !amount.IsPositive() {
		return 0, errors.New("amount must be greater than zero")
	}
	units := amount.Shift(6)
	if !units.Equal(units.Truncate(0)) || units.Sign() <= 0 || !units.IsInteger() {
		return 0, errors.New("amount must have no more than 6 decimal places")
	}
	if !units.BigInt().IsUint64() {
		return 0, errors.New("amount is too large")
	}
	return units.BigInt().Uint64(), nil
}

func directUSDTBaseAmount(amount decimal.Decimal) (decimal.Decimal, error) {
	if operation_setting.GetQuotaDisplayType() != operation_setting.QuotaDisplayTypeTokens {
		return amount, nil
	}
	quotaPerUnit := common.GetQuotaPerUnit()
	if !common.IsValidQuotaPerUnit() {
		return decimal.Zero, errors.New("invalid quota conversion")
	}
	return amount.Div(decimal.NewFromFloat(quotaPerUnit)), nil
}

func directUSDTBaseAmountUnits(amount decimal.Decimal) (uint64, error) {
	baseAmount, err := directUSDTBaseAmount(amount)
	if err != nil {
		return 0, err
	}
	return directUSDTAmountUnits(baseAmount)
}

func RequestDirectUSDTTRC20Pay(c *gin.Context) {
	requestDirectUSDTNetworkPay(c, "TRON", true)
}

func GetDirectUSDTTRC20Status(c *gin.Context) {
	c.Params = append(c.Params, gin.Param{Key: "network", Value: "TRON"})
	GetDirectUSDTNetworkStatus(c)
}
