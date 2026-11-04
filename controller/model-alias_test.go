package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestListModelsPlatformAliasesFollowAvailabilityAndTokenLimits(t *testing.T) {
	withSelfUseModeDisabled(t)
	withTieredBillingConfig(t, map[string]string{
		"zz-alias-canonical": "tiered_expr", "zz-unavailable-canonical": "tiered_expr", "zz-disabled-canonical": "tiered_expr",
	}, map[string]string{
		"zz-alias-canonical": `tier("base", p * 1 + c * 2)`, "zz-unavailable-canonical": `tier("base", p * 1 + c * 2)`, "zz-disabled-canonical": `tier("base", p * 1 + c * 2)`,
	})
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.Model{ModelName: "zz-alias-canonical", Aliases: model.ModelAliases{"zz-cursor-alias"}, Status: 1}).Error)
	require.NoError(t, db.Create(&model.Model{ModelName: "zz-unavailable-canonical", Aliases: model.ModelAliases{"zz-unavailable-alias"}, Status: 1}).Error)
	require.NoError(t, db.Create(&model.Model{ModelName: "zz-disabled-canonical", Aliases: model.ModelAliases{"zz-disabled-alias"}, Status: 0}).Error)
	require.NoError(t, db.Model(&model.Model{}).Where("model_name = ?", "zz-disabled-canonical").Update("status", 0).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 1, Name: "alias-channel", Key: "test", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: ratio_setting.PricingGroupKey("default"), Model: "zz-alias-canonical", ChannelId: 1, Enabled: true}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: ratio_setting.PricingGroupKey("default"), Model: "zz-disabled-canonical", ChannelId: 1, Enabled: true}).Error)
	model.RefreshPricing()

	for _, pricing := range model.GetPricing() {
		require.NotEqual(t, "zz-cursor-alias", pricing.ModelName)
		if pricing.ModelName == "zz-alias-canonical" {
			require.Equal(t, model.ModelAliases{"zz-cursor-alias"}, pricing.Aliases)
			require.Equal(t, "tiered_expr", pricing.BillingMode)
		}
	}

	for _, allowed := range []string{"", "zz-cursor-alias", "zz-alias-canonical"} {
		t.Run("allowed="+allowed, func(t *testing.T) {
			response := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(response)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "default")
			if allowed != "" {
				common.SetContextKey(ctx, constant.ContextKeyTokenModelLimitEnabled, true)
				common.SetContextKey(ctx, constant.ContextKeyTokenModelLimit, map[string]bool{allowed: true})
			}
			ListModels(ctx, constant.ChannelTypeOpenAI)
			ids := decodeListModelsResponse(t, response)
			if allowed == "" || allowed == "zz-cursor-alias" {
				require.Contains(t, ids, "zz-cursor-alias")
				var body listModelsResponse
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
				for _, listed := range body.Data {
					if listed.Id == "zz-cursor-alias" {
						require.Equal(t, "openai", listed.OwnedBy)
						require.Equal(t, model.GetModelSupportEndpointTypes("zz-alias-canonical"), listed.SupportedEndpointTypes)
					}
				}
			} else {
				require.NotContains(t, ids, "zz-cursor-alias")
			}
			require.NotContains(t, ids, "zz-unavailable-alias")
			require.NotContains(t, ids, "zz-disabled-alias")
		})
	}
}
