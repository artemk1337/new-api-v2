package controller

import (
	"crypto/hmac"
	"crypto/sha512"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
