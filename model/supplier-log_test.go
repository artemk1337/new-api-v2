package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestSupplierUserLogHidesProcurementAndFuturePrivateFields(t *testing.T) {
	logs := []*Log{{
		Type: LogTypeError, ChannelId: 12, ChannelName: "private-supplier", UpstreamRequestId: "private-request",
		Content: "upstream https://private-supplier/pricing failed", ModelName: "private-model",
		Other: common.MapToJsonStr(map[string]interface{}{
			"supplier_pricing": true, "requested_model": "gpt", "cache_tokens": 42,
			"model_price": 0.006, "group_ratio": 2, "expr_b64": "secret", "matched_tier": "private-step",
			"raw_error": "private-supplier", "future_private_field": map[string]interface{}{"secret": "private-supplier"},
			"admin_info": map[string]interface{}{"supplier_id": 1}, "frt": 0.2,
		}),
	}}
	formatUserLogs(logs, 0)
	data, err := common.Marshal(logs)
	require.NoError(t, err)
	for _, secret := range []string{"private-", "supplier_pricing", "supplier_id", "model_price", "group_ratio", "expr_b64", "matched_tier", "raw_error", "future_private"} {
		require.NotContains(t, string(data), secret)
	}
	require.Equal(t, "gpt", logs[0].ModelName)
	require.Zero(t, logs[0].ChannelId)
	other, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	require.Equal(t, 42.0, other["cache_tokens"])
	require.Equal(t, "Request failed", logs[0].Content)
}
