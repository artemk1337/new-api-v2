package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoTaskBillingModelNamePrefersPersistedTarget(t *testing.T) {
	task := &model.Task{Properties: model.Properties{
		OriginModelName:  "alias-model",
		BillingModelName: "provider-model",
	}}
	task.SetData(map[string]any{"model": "alias-model"})

	assert.Equal(t, "provider-model", videoTaskBillingModelName(task))
}

func TestVideoTaskBillingModelNameUsesLegacyMappedTarget(t *testing.T) {
	oldPrice := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrice))
	})
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"provider-model":0.5}`))

	task := &model.Task{Properties: model.Properties{OriginModelName: "legacy-alias", UpstreamModelName: "provider-model"}}
	assert.Equal(t, "provider-model", videoTaskBillingModelName(task))
}
