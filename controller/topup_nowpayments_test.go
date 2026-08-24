package controller

import (
	"crypto/hmac"
	"crypto/sha512"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestVerifyNOWPaymentsSignature(t *testing.T) {
	payload := map[string]any{
		"payment_id":     "payment-1",
		"payment_status": "finished",
		"order_id":       "order-1",
	}
	body, err := common.Marshal(payload)
	require.NoError(t, err)
	mac := hmac.New(sha512.New, []byte("secret"))
	_, err = mac.Write(body)
	require.NoError(t, err)

	signature := fmt.Sprintf("%x", mac.Sum(nil))
	assert.True(t, verifyNOWPaymentsSignature(body, signature, "secret"))
	assert.False(t, verifyNOWPaymentsSignature(body, signature, "other-secret"))
	assert.False(t, verifyNOWPaymentsSignature(body, "invalid", "secret"))
}

func TestCompleteNOWPaymentsPaymentAcknowledgesExpiredOrder(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.TopUp{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	require.NoError(t, db.Create(&model.TopUp{TradeNo: "now-expired", PaymentProvider: model.PaymentProviderNOWPayments, Status: common.TopUpStatusExpired}).Error)

	status, err := completeNOWPaymentsPayment(&service.NOWPaymentsPayment{PaymentStatus: "finished", OrderID: "now-expired"}, "127.0.0.1")
	require.NoError(t, err)
	assert.Equal(t, 200, status)
}
