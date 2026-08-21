package relay

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyGeminiNoThinkingModelUsesMappedBillingTarget(t *testing.T) {
	oldRatio := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio))
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"provider-model-nothinking":2}`))

	info := &relaycommon.RelayInfo{
		OriginModelName:  "alias-model",
		BillingModelName: "provider-model",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "provider-model",
			IsModelMapped:     true,
		},
	}
	applyGeminiNoThinkingModel(info)

	assert.Equal(t, "alias-model", info.OriginModelName)
	assert.Equal(t, "provider-model-nothinking", info.UpstreamModelName)
	assert.Equal(t, "provider-model-nothinking", info.BillingModelName)
}
