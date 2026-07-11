package service

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestQuotaNotifyContentForTelegram(t *testing.T) {
	content, values := quotaNotifyContent(dto.NotifyTypeTelegram, "Quota warning", "42", "https://example.com/topup")

	require.Equal(t, "{{value}}，当前剩余额度为 {{value}}，为了不影响您的使用，请及时充值。\n充值链接：{{value}}", content)
	require.Equal(t, []interface{}{"Quota warning", "42", "https://example.com/topup"}, values)
}
