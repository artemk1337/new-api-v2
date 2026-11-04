package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestForcedUnboundChannelPreservesLegacyUnlistedModels(t *testing.T) {
	for _, test := range []struct {
		name        string
		bound       bool
		listed      bool
		mismatch    bool
		priced      bool
		wantSuccess bool
	}{
		{name: "unbound unlisted globally priced", priced: true, wantSuccess: true},
		{name: "unbound unlisted missing global price"},
		{name: "bound unlisted", bound: true, priced: true},
		{name: "bound listed missing supplier tariff", bound: true, listed: true, priced: true},
		{name: "bound mismatched profile", bound: true, listed: true, mismatch: true, priced: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := setupModelListControllerTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.Log{}))
			previousGroups := ratio_setting.PricingGroups2JSONString()
			previousPrices := ratio_setting.ModelPrice2JSONString()
			previousCountTokens, previousCache := constant.CountToken, common.MemoryCacheEnabled
			t.Cleanup(func() {
				require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(previousGroups))
				require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousPrices))
				constant.CountToken, common.MemoryCacheEnabled = previousCountTokens, previousCache
			})
			constant.CountToken, common.MemoryCacheEnabled = false, false
			// A free group avoids unrelated funding setup while its model still
			// requires an explicit positive global billing contract.
			require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[{"id":1,"name":"default","ratio":0,"selectable":true}]`))
			priceJSON := `{}`
			if test.priced {
				priceJSON = `{"zz-forced-legacy-model":0.01}`
			}
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(priceJSON))
			require.NoError(t, db.Create(&model.User{Id: 1, Username: "forced-route-user", Status: common.UserStatusEnabled, Group: "default", Quota: 100000}).Error)
			var requests atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				require.Equal(t, "Bearer forced-fixture-key", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"chat_legacy","object":"chat.completion","model":"zz-forced-legacy-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
			}))
			defer upstream.Close()
			service.InitHttpClient()
			channel := model.Channel{Id: 1, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled,
				Key: "forced-fixture-key", Group: "1", Models: "configured-model", BaseURL: &upstream.URL}
			if test.bound {
				supplierID, profileID := 1, 1
				require.NoError(t, db.Create(&model.Supplier{ID: supplierID, Code: "forced", Name: "Forced", Adapter: "manual", Enabled: true}).Error)
				for _, id := range []int{1, 2} {
					require.NoError(t, db.Create(&model.SupplierPricingProfile{ID: id, SupplierID: supplierID, Code: string(rune('a' + id)), Enabled: true, USDPerCredit: 1, ConfigRevision: 1}).Error)
				}
				channel.SupplierID, channel.SupplierProfileID = &supplierID, &profileID
				bindingProfileID := 1
				if test.mismatch {
					bindingProfileID = 2
				}
				require.NoError(t, db.Create(&model.PricingGroupSupplierBinding{GroupID: 1, SupplierID: supplierID, ProfileID: bindingProfileID}).Error)
			}
			require.NoError(t, db.Create(&channel).Error)
			if test.listed {
				require.NoError(t, db.Create(&model.Ability{Group: "1", Model: "zz-forced-legacy-model", ChannelId: channel.Id, Enabled: true}).Error)
			}
			router := gin.New()
			router.Use(func(c *gin.Context) {
				common.SetContextKey(c, constant.ContextKeyUserId, 1)
				common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
				common.SetContextKey(c, constant.ContextKeyUserQuota, 100000)
				common.SetContextKey(c, constant.ContextKeyTokenUnlimited, true)
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "1")
				common.SetContextKey(c, constant.ContextKeyTokenGroup, "1")
				common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, "1")
				c.Next()
			})
			router.POST("/v1/chat/completions", middleware.Distribute(), func(c *gin.Context) { Relay(c, types.RelayFormatOpenAI) })
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"zz-forced-legacy-model","messages":[{"role":"user","content":"hi"}],"max_tokens":1}`))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			if test.wantSuccess {
				require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
				require.Contains(t, recorder.Body.String(), `"content":"ok"`)
				require.EqualValues(t, 1, requests.Load())
			} else {
				require.GreaterOrEqual(t, recorder.Code, http.StatusBadRequest, recorder.Body.String())
				require.Zero(t, requests.Load())
			}
		})
	}
}
