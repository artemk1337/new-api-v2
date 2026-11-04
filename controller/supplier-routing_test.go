package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGroupBillingModelsResolveAliasesIndependently(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	previousGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(previousGroups)) })
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"second","ratio":2,"selectable":true}
	]`))
	for index, mapping := range []string{`{"alias":"upstream-first"}`, `{"alias":"upstream-second"}`} {
		channel := model.Channel{Id: index + 1, Status: common.ChannelStatusEnabled, ModelMapping: &mapping}
		require.NoError(t, db.Create(&channel).Error)
		group := []string{"1", "2"}[index]
		require.NoError(t, db.Create(&model.Ability{Group: group, Model: "alias", ChannelId: channel.Id, Enabled: true}).Error)
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{OriginModelName: "alias", RequestedModelName: "alias"}

	groups, err := resolveGroupBillingModels(ctx, info, []string{"1", "2"})

	require.NoError(t, err)
	require.Equal(t, []string{"1", "2"}, groups)
	require.Equal(t, map[string]string{"1": "upstream-first", "2": "upstream-second"}, info.GroupBillingModelNames)
}

func TestQuotedAutoOrderUsesTotalCostAndKeepsMaximumReserve(t *testing.T) {
	for _, cheapestOnly := range []bool{false, true} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		info := &relaycommon.RelayInfo{AutoRoute: relaycommon.AutoRouteState{
			ReservedQuota: 1000,
			ReserveGroup:  "low-multiplier",
			Candidates: []relaycommon.AutoRouteCandidate{
				{Group: "low-multiplier", Ratio: 0.01, EstimatedQuota: 1000},
				{Group: "low-total-cost", Ratio: 2, EstimatedQuota: 100},
			},
		}}

		require.NoError(t, orderQuotedAutoRoute(ctx, info, cheapestOnly))

		require.Equal(t, "low-total-cost", info.UsingGroup)
		require.Equal(t, "low-total-cost", info.AutoRoute.InitialGroup)
		require.Equal(t, "low-total-cost", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
		if cheapestOnly {
			require.Equal(t, 100, info.AutoRoute.ReservedQuota)
			require.Len(t, info.AutoRoute.Candidates, 1)
		} else {
			require.Equal(t, 1000, info.AutoRoute.ReservedQuota)
			require.Equal(t, []string{"low-total-cost", "low-multiplier"}, common.GetContextKeyStringSlice(ctx, constant.ContextKeyAutoGroupSnapshot))
		}
	}
}

func TestFrozenRouteRejectsProfileRebindingBeforeDispatch(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Supplier{}, &model.SupplierPricingProfile{}, &model.PricingGroupSupplierBinding{}))
	require.NoError(t, db.Create(&model.Supplier{ID: 1, Code: "supplier", Name: "Private supplier", Adapter: "newapi", Enabled: true}).Error)
	for _, profileID := range []int{10, 20} {
		require.NoError(t, db.Create(&model.SupplierPricingProfile{ID: profileID, SupplierID: 1, Code: string(rune('a' + profileID)), Enabled: true, USDPerCredit: 1}).Error)
	}
	require.NoError(t, db.Create(&model.PricingGroupSupplierBinding{GroupID: 1, SupplierID: 1, ProfileID: 10}).Error)
	supplierID, profileID := 1, 10
	channel := &model.Channel{Id: 42, Status: common.ChannelStatusEnabled, SupplierID: &supplierID, SupplierProfileID: &profileID}
	require.NoError(t, db.Create(channel).Error)
	info := &relaycommon.RelayInfo{
		UsingGroup: "1", OriginModelName: "model", ForcePreConsume: true,
		PriceData: types.PriceData{SupplierTariff: &types.SupplierModelTariff{SupplierID: 1, ProfileID: 10, Revision: 3}},
	}
	require.NoError(t, validateFrozenRoute(channel, info))

	// Both live resources now agree with each other, but the request reserved
	// a different procurement profile and must stop before dispatch.
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 42).Update("supplier_profile_id", 20).Error)
	require.NoError(t, db.Model(&model.PricingGroupSupplierBinding{}).Where("group_id = ?", 1).Update("profile_id", 20).Error)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("channel_id", 42)
	selected, routeErr := getChannel(ctx, info, &service.RetryParam{Ctx: ctx})

	require.Nil(t, selected)
	require.NotNil(t, routeErr)
	require.NotContains(t, routeErr.Error(), "Private supplier")
}

func TestFrozenRouteRejectsChangedModelMapping(t *testing.T) {
	info := &relaycommon.RelayInfo{OriginModelName: "alias", BillingModelName: "reserved-target"}
	require.NoError(t, validateFrozenRouteMapping(`{"alias":"reserved-target"}`, info))
	require.ErrorContains(t, validateFrozenRouteMapping(`{"alias":"other-target"}`, info), "reserved model")
}

func TestFrozenCompactRouteKeepsCompactTariffWithoutChannelAlias(t *testing.T) {
	info := &relaycommon.RelayInfo{OriginModelName: "model-compact", RelayMode: relayconstant.RelayModeResponsesCompact}
	info.OriginModelName = ratio_setting.WithCompactModelSuffix("model")
	require.NoError(t, validateFrozenRouteMapping(`{}`, info))
}

func TestQuotedRouteKeepsCompatibleAffinityChannelAndSelectedKey(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.PricingGroupSupplierBinding{}))
	require.NoError(t, db.Create(&model.Channel{Id: 42, Status: common.ChannelStatusEnabled, Key: "next-key"}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "1", Model: "model", ChannelId: 42, Enabled: true}).Error)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(ctx, constant.ContextKeyChannelId, 42)
	common.SetContextKey(ctx, constant.ContextKeyChannelKey, "affinity-selected-key")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "1")
	info := &relaycommon.RelayInfo{UsingGroup: "1", OriginModelName: "model", ForcePreConsume: true}

	require.NoError(t, setupQuotedInitialChannel(ctx, info))

	require.Equal(t, 42, common.GetContextKeyInt(ctx, constant.ContextKeyChannelId))
	require.Equal(t, "affinity-selected-key", common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
}

func TestSupplierPublicRelayErrorDropsProviderMetadata(t *testing.T) {
	err := types.WithOpenAIError(types.OpenAIError{
		Message: "private-supplier.example: exhausted procurement credit",
		Type:    "private_supplier_error", Code: "supplier_private_code",
		Metadata: []byte(`{"supplier_id":7,"cost":0.006}`),
	}, http.StatusTooManyRequests)

	public := publicSupplierRelayError(err)
	wire, marshalErr := common.Marshal(public.ToOpenAIError())

	require.NoError(t, marshalErr)
	require.Equal(t, http.StatusTooManyRequests, public.StatusCode)
	require.NotContains(t, string(wire), "private")
	require.NotContains(t, string(wire), "supplier_id")
	require.NotContains(t, string(wire), "0.006")
	require.Contains(t, err.Error(), "private-supplier.example")
}

func TestSupplierErrorLogsHideProcurementForTokenReaders(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	previous := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = true
	t.Cleanup(func() { constant.ErrorLogEnabled = previous })
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set("id", 1)
	ctx.Set("token_id", 77)
	ctx.Set("original_model", "public-model")
	ctx.Set("supplier_pricing", true)
	info := &relaycommon.RelayInfo{PriceData: types.PriceData{SupplierTariff: &types.SupplierModelTariff{SupplierID: 1, ProfileID: 2}}}

	processChannelError(ctx, types.ChannelError{ChannelId: 9, ChannelName: "secret-supplier"}, types.NewErrorWithStatusCode(
		errors.New("secret procurement failure"), types.ErrorCodeBadResponse, http.StatusBadGateway,
	), info)

	var persisted model.Log
	require.NoError(t, db.First(&persisted).Error)
	var internal map[string]any
	require.NoError(t, common.UnmarshalJsonStr(persisted.Other, &internal))
	require.Equal(t, true, internal["supplier_pricing"])
	logs, err := model.GetLogByTokenId(77)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.Zero(t, logs[0].ChannelId)
	require.Equal(t, "Request failed", logs[0].Content)
	require.NotContains(t, logs[0].Other, "secret")
	require.NotContains(t, logs[0].Other, "supplier")
}
