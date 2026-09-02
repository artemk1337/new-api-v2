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
	if _, allowed := directCryptoMethodForUser(c); !allowed || !operation_setting.IsPaymentComplianceConfirmed() {
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
	topUp := &model.TopUp{UserId: userID, TradeNo: tradeNo, Amount: int64(baseAmount), RequestedAmount: baseAmount, PaymentMethod: model.DirectCryptoProvider, PaymentMethodName: "Crypto", PaymentProvider: model.DirectCryptoProvider, QuotaToAdd: quotaToAdd, CreateTime: now, Status: common.TopUpStatusPending}
	service.ApplyPaymentSnapshot(topUp, "USD", 1, baseAmount, 1, baseAmount)
	// ApplyPaymentSnapshot captures the immutable paid principal. Keep the
	// user-specific effective cashback calculated above as the total credit.
	topUp.QuotaToAdd = quotaToAdd
	contract := setting.USDTTRC20Contract
	if network == "TON" {
		contract = setting.USDTTONJettonMaster
	}
	payment := &model.DirectCryptoPayment{TradeNo: tradeNo, UserId: userID, Network: network, Token: "USDT", Address: address, ReceivingOwner: address, Destination: address, BaseUnits: baseUnits, Contract: contract, Status: model.DirectCryptoPending, ExpiresAt: now + int64(operation_setting.PendingTopUpTTL(model.DirectCryptoProvider)/time.Second), CreatedAt: now, UpdatedAt: now}
	if network == "SOLANA" {
		payment.Contract = setting.USDTSolanaMint
		payment.Destination = setting.USDTSolanaReceivingTokenAccount
	}
	if err := model.CreateDirectUSDTOrder(topUp, payment); err != nil {
		common.SysError("create direct crypto order failed: " + err.Error())
		common.ApiErrorMsg(c, "Failed to create order")
		return
	}
	common.ApiSuccess(c, gin.H{"payment_url": "/crypto/" + strings.ToLower(network) + "/" + tradeNo, "trade_no": tradeNo, "network": network, "token": "USDT", "receiving_address": payment.Address, "destination_token_account": payment.Destination, "amount": model.DirectUSDTAmountString(payment.ExpectedUnits), "expires_at": payment.ExpiresAt})
}

func GetDirectUSDTNetworkStatus(c *gin.Context) {
	requestedNetwork := strings.ToUpper(strings.TrimSpace(c.Param("network")))
	if requestedNetwork != "TRON" && requestedNetwork != "TON" && requestedNetwork != "SOLANA" {
		c.Status(http.StatusNotFound)
		return
	}
	// Status is authorized by the parent method and the immutable invoice owner.
	// Do not perform live RPC/readiness checks here: a key rotation or a slow
	// Solana RPC must not make an already issued invoice unreadable every poll.
	// New invoice creation remains gated by DirectUSDTNetworkIsReady.
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
	common.ApiSuccess(c, gin.H{"trade_no": payment.TradeNo, "status": payment.Status, "network": payment.Network, "token": "USDT", "receiving_address": payment.Address, "destination_token_account": payment.Destination, "amount": model.DirectUSDTAmountString(payment.ExpectedUnits), "expires_at": payment.ExpiresAt})
}

func hasPaymentMethodType(methods []map[string]string, provider string) bool {
	for _, m := range methods {
		if m != nil && strings.EqualFold(strings.TrimSpace(m["type"]), provider) {
			return true
		}
	}
	return false
}
