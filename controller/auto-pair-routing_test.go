package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAutoRelayRetriesInRetailPairBenefitOrderAndSkipsUnavailableGroups(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.Log{}))
	previousGroups := ratio_setting.PricingGroups2JSONString()
	previousSpecial := ratio_setting.GroupGroupRatio2JSONString()
	previousCountTokens, previousCache := constant.CountToken, common.MemoryCacheEnabled
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(previousGroups))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(previousSpecial))
		constant.CountToken, common.MemoryCacheEnabled = previousCountTokens, previousCache
	})
	constant.CountToken, common.MemoryCacheEnabled = false, false
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":2,"selectable":true},
		{"id":2,"name":"supplier","ratio":0.1,"selectable":true},
		{"id":3,"name":"disabled","ratio":1,"selectable":true},
		{"id":4,"name":"unpublished","ratio":1,"selectable":true}
	]`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"member":{"default":0.5,"supplier":3}}`))
	withTieredBillingConfig(t, map[string]string{"global-price": "tiered_expr", "disabled-price": "tiered_expr"}, map[string]string{
		"global-price": "p * 4 + c * 100", "disabled-price": "p * 0.01 + c * 0.01",
	})
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "pair-order-user", Status: common.UserStatusEnabled, Group: "member", Quota: 1000000}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 1, UserId: 1, Key: "pair-order-token", Status: common.TokenStatusEnabled, UnlimitedQuota: true}).Error)
	require.NoError(t, db.Create(&model.Supplier{ID: 1, Code: "private-source", Name: "Private", Adapter: "manual", Enabled: true}).Error)
	for _, id := range []int{1, 2} {
		require.NoError(t, db.Create(&model.SupplierPricingProfile{ID: id, SupplierID: 1, Code: string(rune('a' + id)), Enabled: true, USDPerCredit: 1, ConfigRevision: 1}).Error)
	}
	input, output := 1.0, 0.1
	tariff := types.SupplierModelTariff{ModelName: "supplier-price", BillingMode: types.SupplierBillingModeToken, PriceUnit: "million_tokens", InputUSD: &input, OutputUSD: &output}
	data, err := common.Marshal(tariff)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.SupplierPricingState{ProfileID: 1, ActiveRevision: 1}).Error)
	require.NoError(t, db.Create(&model.SupplierPricingRevision{ProfileID: 1, Revision: 1, ConfigRevision: 1}).Error)
	require.NoError(t, db.Create(&model.SupplierModelPrice{ProfileID: 1, Revision: 1, ModelName: tariff.ModelName, Data: string(data)}).Error)
	require.NoError(t, db.Create(&[]model.PricingGroupSupplierBinding{{GroupID: 2, SupplierID: 1, ProfileID: 1}, {GroupID: 4, SupplierID: 1, ProfileID: 2}}).Error)
	var mu sync.Mutex
	var requests []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		mu.Lock()
		requests = append(requests, key)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if key == "supplier-key" {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"busy","type":"rate_limit_error"},"billing":{"billed":false}}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"chat_pair_order","object":"chat.completion","model":"global-price","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":1,"total_tokens":11}}`))
	}))
	defer upstream.Close()
	service.InitHttpClient()
	for index, fixture := range []struct {
		key     string
		target  string
		enabled bool
		profile int
	}{
		{"global-key", "global-price", true, 0},
		{"supplier-key", "supplier-price", true, 1},
		{"disabled-key", "disabled-price", false, 0},
		{"unpublished-key", "supplier-price", true, 2},
	} {
		id := index + 1
		mapping := `{"route-alias":"` + fixture.target + `"}`
		status := common.ChannelStatusEnabled
		if !fixture.enabled {
			status = common.ChannelStatusManuallyDisabled
		}
		channel := model.Channel{Id: id, Type: constant.ChannelTypeOpenAI, Status: status, Key: fixture.key, Models: "route-alias", BaseURL: &upstream.URL, ModelMapping: &mapping}
		group := []string{"1", "2", "3", "4"}[index]
		channel.Group = group
		if fixture.profile > 0 {
			supplierID, profileID := 1, fixture.profile
			channel.SupplierID, channel.SupplierProfileID = &supplierID, &profileID
		}
		require.NoError(t, db.Create(&channel).Error)
		require.NoError(t, db.Create(&model.Ability{Group: group, Model: "route-alias", ChannelId: id, Enabled: fixture.enabled}).Error)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUserId, 1)
		common.SetContextKey(c, constant.ContextKeyUserGroup, "member")
		common.SetContextKey(c, constant.ContextKeyUserQuota, 1000000)
		common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
		common.SetContextKey(c, constant.ContextKeyTokenId, 1)
		common.SetContextKey(c, constant.ContextKeyTokenKey, "pair-order-token")
		common.SetContextKey(c, constant.ContextKeyTokenUnlimited, true)
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "auto")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "auto")
		common.SetContextKey(c, constant.ContextKeyTokenAutoGroupCandidates, []string{"3", "4", "2", "1"})
		c.Next()
	})
	var quotedGroups []string
	router.POST("/v1/chat/completions", middleware.Distribute(), func(c *gin.Context) {
		Relay(c, types.RelayFormatOpenAI)
		quotedGroups = common.GetContextKeyStringSlice(c, constant.ContextKeyAutoGroupSnapshot)
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"route-alias","messages":[{"role":"user","content":"hi"}],"max_tokens":10}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"content":"ok"`)
	mu.Lock()
	actualRequests := append([]string(nil), requests...)
	mu.Unlock()
	require.Equal(t, []string{"supplier-key", "global-key"}, actualRequests)
	require.Equal(t, []string{"2", "1"}, quotedGroups)
}
