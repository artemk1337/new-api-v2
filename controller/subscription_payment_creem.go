package controller

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/thanhpk/randstr"
)

type SubscriptionCreemPayRequest struct {
	PlanId int `json:"plan_id"`
}

func SubscriptionRequestCreemPay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}
	if !paymentMethodAllowedForUser(c, model.PaymentMethodCreem) {
		return
	}

	var req SubscriptionCreemPayRequest

	// Keep body for debugging consistency (like RequestCreemPay)
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 订阅支付请求读取失败 error=%q", err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "read query error"})
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	plan, err := model.GetSubscriptionPlanById(req.PlanId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !plan.Enabled {
		common.ApiErrorMsg(c, "套餐未启用")
		return
	}
	if plan.CreemProductId == "" {
		common.ApiErrorMsg(c, "该套餐未配置 CreemProductId")
		return
	}
	config := setting.GetCreemConfig()
	if strings.TrimSpace(config.WebhookSecret) == "" {
		common.ApiErrorMsg(c, "Creem Webhook 未配置")
		return
	}

	userId := c.GetInt("id")
	user, err := model.GetUserById(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if user == nil {
		common.ApiErrorMsg(c, "用户不存在")
		return
	}

	reference := "sub-creem-ref-" + randstr.String(6)
	referenceId := "sub_ref_" + common.Sha1([]byte(reference+time.Now().String()+user.Username))
	configuredProduct, err := creemConfiguredProductFromConfig(config, plan.CreemProductId)
	if err != nil {
		common.ApiErrorMsg(c, "Creem 产品配置不存在")
		return
	}
	providerProduct, err := creemProductLookup(withCreemConfig(c.Request.Context(), config), plan.CreemProductId)
	if err != nil {
		common.ApiErrorMsg(c, "Creem 产品暂不可用")
		return
	}
	if err := validateCreemProductContract(configuredProduct, providerProduct); err != nil {
		common.ApiErrorMsg(c, "Creem 产品价格或货币不匹配")
		return
	}
	planProduct := &CreemProduct{ProductId: plan.CreemProductId, Price: plan.PriceAmount, Currency: configuredProduct.Currency}
	if err := validateCreemProductContract(planProduct, providerProduct); err != nil {
		common.ApiErrorMsg(c, "Creem 产品价格或货币与套餐不匹配")
		return
	}
	currency := strings.ToUpper(strings.TrimSpace(configuredProduct.Currency))
	if !model.SubscriptionProviderAmountRepresentable(plan.PriceAmount, currency) {
		common.ApiErrorMsg(c, "сумма тарифа должна быть указана с точностью до минимальной единицы валюты")
		return
	}

	// create pending order first
	order := &model.SubscriptionOrder{
		UserId:            userId,
		PlanId:            plan.Id,
		Money:             plan.PriceAmount,
		TradeNo:           referenceId,
		PaymentMethod:     model.PaymentMethodCreem,
		PaymentMethodName: model.PaymentMethodDisplayName(model.PaymentMethodCreem),
		PaymentProvider:   model.PaymentProviderCreem,
		CreateTime:        time.Now().Unix(),
		Status:            common.TopUpStatusPending,
	}
	order.PlanSnapshot, err = model.NewSubscriptionOrderSnapshotWithProvider(plan, model.PaymentProviderCreem, plan.PriceAmount, currency, plan.CreemProductId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.CreatePendingSubscriptionOrder(order, plan.MaxPurchasePerUser); err != nil {
		if errors.Is(err, model.ErrSubscriptionPurchaseLimit) {
			common.ApiErrorMsg(c, err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	// Reuse Creem checkout generator by building a lightweight product reference.
	product := &CreemProduct{
		ProductId: plan.CreemProductId,
		Name:      plan.Title,
		Price:     plan.PriceAmount,
		Currency:  currency,
		Quota:     0,
	}

	checkoutUrl, err := genCreemLink(c.Request.Context(), config, referenceId, product, user.Email, user.Username)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 订阅支付链接创建失败 trade_no=%s product_id=%s error=%q", referenceId, product.ProductId, err.Error()))
		if isPermanentCreemCreateError(err) {
			_ = model.FailSubscriptionOrder(referenceId, model.PaymentProviderCreem)
		}
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"checkout_url": checkoutUrl,
			"order_id":     referenceId,
		},
	})
}

func creemConfiguredProductCurrency(productID string) string {
	product, err := creemConfiguredProductFromConfig(setting.GetCreemConfig(), productID)
	if err == nil {
		return strings.ToUpper(strings.TrimSpace(product.Currency))
	}
	return "UNKNOWN"
}

func creemConfiguredProduct(productID string) (*CreemProduct, error) {
	return creemConfiguredProductFromConfig(setting.GetCreemConfig(), productID)
}

func creemConfiguredProductFromConfig(config setting.CreemConfig, productID string) (*CreemProduct, error) {
	var products []CreemProduct
	if err := common.Unmarshal([]byte(config.Products), &products); err != nil {
		return nil, fmt.Errorf("invalid Creem products configuration: %w", err)
	}
	for i := range products {
		if products[i].ProductId == productID && strings.TrimSpace(products[i].Currency) != "" {
			return &products[i], nil
		}
	}
	return nil, fmt.Errorf("Creem product %s is not configured", productID)
}
