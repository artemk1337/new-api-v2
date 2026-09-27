package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func RequestDirectUSDTNetworkPay(c *gin.Context) {
	network := strings.ToUpper(strings.TrimSpace(c.Param("network")))
	requestDirectUSDTNetworkPay(c, network, false)
}

// requestDirectUSDTNetworkPay creates all new invoices under crypto_direct.
// legacyTRON accepts the historical body provider only on the legacy route.
func requestDirectUSDTNetworkPay(c *gin.Context, network string, legacyTRON bool) {
	if network != "TON" && network != "SOLANA" && network != "TRON" {
		common.ApiErrorMsg(c, "Unsupported USDT network")
		return
	}
	directMethod, allowed := directCryptoMethodForUser(c)
	if !allowed || !operation_setting.IsPaymentComplianceConfirmed() {
		common.ApiErrorMsg(c, "USDT payments are not available")
		return
	}
	if !model.IsDirectUSDTNetworkMethodConfigured(model.DirectCryptoProvider) || !model.DirectUSDTNetworkIsReady(network) {
		common.ApiErrorMsg(c, "USDT payments are not available")
		return
	}
	address := setting.USDTTRC20ReceivingAddress
	if network == "TON" {
		address = setting.USDTTONReceivingAddress
	}
	if network == "SOLANA" {
		address = setting.USDTSolanaReceivingAddress
	}
	var req DirectUSDTTopUpRequest
	if err := c.ShouldBindJSON(&req); err != nil ||
		(!strings.EqualFold(strings.TrimSpace(req.PaymentMethod), model.DirectCryptoProvider) &&
			!(legacyTRON && strings.EqualFold(strings.TrimSpace(req.PaymentMethod), model.DirectUSDTTRC20Provider))) {
		common.ApiErrorMsg(c, "Invalid parameters")
		return
	}
	baseAmountDecimal, err := directUSDTBaseAmount(req.Amount)
	if err != nil {
		common.ApiErrorMsg(c, "Invalid quota conversion")
		return
	}
	baseUnits, err := directUSDTAmountUnits(baseAmountDecimal)
	if err != nil || baseUnits < directUSDTBaseUnits {
		common.ApiErrorMsg(c, "Top-up amount cannot be less than $10")
		return
	}
	// Snapshot the checkout rounding policy before calculating quota. The same
	// normalized principal is later persisted by the model and cannot drift if
	// an operator changes the setting while the request is in flight.
	roundToCents := setting.USDTTRC20RoundToCents
	baseUnits = model.NormalizeDirectUSDTBaseUnits(baseUnits, roundToCents)
	baseAmountDecimal = decimal.NewFromUint64(baseUnits).Shift(-6)
	userID := c.GetInt("id")
	if userID == 0 {
		c.Status(http.StatusUnauthorized)
		return
	}
	if _, err := model.GetUserById(userID, false); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorMsg(c, "User does not exist")
		} else {
			common.ApiErrorMsg(c, "Failed to load user")
		}
		return
	}
	baseAmount := decimal.NewFromUint64(baseUnits).Shift(-6).InexactFloat64()
	quotaToAdd, err := service.CalculateTopUpQuotaForUser(baseAmount, 0, userID)
	if err != nil {
		common.ApiErrorMsg(c, "Failed to calculate top-up cashback")
		return
	}
	if quotaToAdd <= 0 {
		common.ApiErrorMsg(c, "Top-up amount is too low")
		return
	}
	now := time.Now().Unix()
	tradeNo := fmt.Sprintf("%s%d%s", strings.ToUpper(network), userID, common.GetRandomString(20))
	methodName := strings.TrimSpace(directMethod["name"])
	if methodName == "" {
		methodName = model.PaymentMethodDisplayName(model.DirectCryptoProvider)
	}
	topUp := &model.TopUp{UserId: userID, TradeNo: tradeNo, Amount: int64(baseAmount), RequestedAmount: baseAmount, PaymentMethod: model.DirectCryptoProvider, PaymentMethodName: methodName, PaymentProvider: model.DirectCryptoProvider, QuotaToAdd: quotaToAdd, CreateTime: now, Status: common.TopUpStatusPending}
	service.ApplyPaymentSnapshot(topUp, "USD", 1, baseAmount, 1, baseAmount)
	// ApplyPaymentSnapshot captures the immutable paid principal. Keep the
	// user-specific effective cashback calculated above as the total credit.
	topUp.QuotaToAdd = quotaToAdd
	contract := setting.USDTTRC20Contract
	if network == "TON" {
		contract = setting.USDTTONJettonMaster
	}
	initialTTL := operation_setting.PendingTopUpTTL(model.DirectCryptoProvider)
	if initialTTL > 24*time.Hour {
		initialTTL = 24 * time.Hour
	}
	payment := &model.DirectCryptoPayment{TradeNo: tradeNo, UserId: userID, Network: network, Token: "USDT", Address: address, ReceivingOwner: address, Destination: address, BaseUnits: baseUnits, RoundToCents: roundToCents, RoundPolicyCaptured: true, Contract: contract, Status: model.DirectCryptoPending, ExpiresAt: now + int64(initialTTL/time.Second), CreatedAt: now, UpdatedAt: now}
	if network == "SOLANA" {
		payment.Contract = setting.USDTSolanaMint
		payment.Destination = setting.USDTSolanaReceivingTokenAccount
	}
	if err := model.CreateDirectUSDTOrder(topUp, payment); err != nil {
		common.SysError("create direct crypto order failed: " + err.Error())
		if errors.Is(err, model.ErrDirectPaymentDuplicatePending) {
			common.ApiErrorMsg(c, "You already have a pending USDT payment for this amount. Open or cancel the previous payment first.")
			return
		}
		if errors.Is(err, model.ErrDirectPaymentAmountExhausted) {
			common.ApiErrorMsg(c, "No receiving wallet is available for this amount right now. Try again later or choose another amount.")
			return
		}
		common.ApiErrorMsg(c, "Failed to create order")
		return
	}
	common.ApiSuccess(c, gin.H{"payment_url": "/crypto/" + strings.ToLower(network) + "/" + tradeNo, "trade_no": tradeNo, "network": network, "token": "USDT", "token_contract": payment.Contract, "address": payment.Address, "receiving_address": payment.Address, "destination_token_account": payment.Destination, "amount": model.DirectUSDTAmountString(payment.ExpectedUnits), "expires_at": payment.ExpiresAt})
}

func GetDirectUSDTNetworkStatus(c *gin.Context) {
	requestedNetwork := strings.ToUpper(strings.TrimSpace(c.Param("network")))
	if requestedNetwork != "TRON" && requestedNetwork != "TON" && requestedNetwork != "SOLANA" {
		c.Status(http.StatusNotFound)
		return
	}
	// PayMethods is authoritative for all public direct-crypto endpoints. The
	// immutable invoice snapshot still avoids live readiness checks, so wallet
	// rotation or a slow RPC cannot break polling while the method is enabled.
	if _, allowed := directCryptoMethodForUser(c); !allowed || !operation_setting.IsPaymentComplianceConfirmed() {
		return
	}
	tradeNo := strings.TrimSpace(c.Param("trade_no"))
	payment, err := model.GetDirectCryptoPayment(tradeNo)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	if payment.UserId != c.GetInt("id") {
		c.Status(http.StatusNotFound)
		return
	}
	if !strings.EqualFold(strings.TrimSpace(payment.Network), requestedNetwork) {
		c.Status(http.StatusNotFound)
		return
	}
	payment, err = model.GetDirectCryptoPaymentStatus(tradeNo, time.Now().Unix())
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	common.ApiSuccess(c, gin.H{"trade_no": payment.TradeNo, "status": payment.Status, "network": payment.Network, "token": "USDT", "token_contract": payment.Contract, "address": payment.Address, "receiving_address": payment.Address, "destination_token_account": payment.Destination, "amount": model.DirectUSDTAmountString(payment.ExpectedUnits), "expires_at": payment.ExpiresAt})
}

func CancelDirectUSDTNetworkPayment(c *gin.Context) {
	network := strings.ToUpper(strings.TrimSpace(c.Param("network")))
	if network != "TRON" && network != "TON" && network != "SOLANA" {
		c.Status(http.StatusNotFound)
		return
	}
	if _, allowed := directCryptoMethodForUser(c); !allowed || !operation_setting.IsPaymentComplianceConfirmed() {
		return
	}
	tradeNo := strings.TrimSpace(c.Param("trade_no"))
	payment, err := model.GetDirectCryptoPayment(tradeNo)
	if err != nil || payment.UserId != c.GetInt("id") || !strings.EqualFold(payment.Network, network) {
		c.Status(http.StatusNotFound)
		return
	}
	if err := model.CancelDirectCryptoPayment(tradeNo, c.GetInt("id")); err != nil {
		if errors.Is(err, model.ErrDirectPaymentAlreadySettled) {
			common.ApiErrorMsg(c, "Payment is already completed")
		} else {
			common.ApiError(c, err)
		}
		return
	}
	common.ApiSuccess(c, gin.H{"status": model.DirectCryptoCancelled, "trade_no": tradeNo})
}

func hasPaymentMethodType(methods []map[string]string, provider string) bool {
	for _, m := range methods {
		if m != nil && strings.EqualFold(strings.TrimSpace(m["type"]), provider) {
			return true
		}
	}
	return false
}
