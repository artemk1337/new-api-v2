package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
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

func TestAutoBillingModelsSkipAmbiguousGroupButRejectInvalidCallerMapping(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	previousGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(previousGroups)) })
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[{"id":1,"name":"default","ratio":1},{"id":2,"name":"ambiguous","ratio":2}]`))
	for index, mapping := range []string{`{"alias":"first"}`, `{"alias":"second"}`, `{"alias":"third"}`} {
		channel := model.Channel{Id: index + 1, Status: common.ChannelStatusEnabled, ModelMapping: &mapping}
		require.NoError(t, db.Create(&channel).Error)
		group := "2"
		if index == 0 {
			group = "1"
		}
		require.NoError(t, db.Create(&model.Ability{Group: group, Model: "alias", ChannelId: channel.Id, Enabled: true}).Error)
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	for _, mode := range []string{"", "auto"} {
		info := &relaycommon.RelayInfo{TokenGroup: mode, OriginModelName: "alias", RequestedModelName: "alias"}
		groups, err := resolveGroupBillingModels(ctx, info, []string{"2", "1"})
		require.NoError(t, err)
		require.Equal(t, []string{"1"}, groups)
		require.Equal(t, map[string]string{"1": "first"}, info.GroupBillingModelNames)
	}
	_, err := resolveGroupBillingModels(ctx, &relaycommon.RelayInfo{TokenGroup: "2", OriginModelName: "alias", RequestedModelName: "alias"}, []string{"2"})
	require.Error(t, err)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelMapping, `{"alias":"mapped","mapped":"alias"}`)
	_, err = resolveGroupBillingModels(ctx, &relaycommon.RelayInfo{TokenGroup: "auto", OriginModelName: "alias", RequestedModelName: "alias"}, []string{"1", "2"})
	require.ErrorContains(t, err, "cycle")
}

func TestTaskRoutePreviewSkipsUnpublishedSupplierPrices(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	previousGroups := ratio_setting.PricingGroups2JSONString()
	previousPrices := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(previousGroups))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousPrices))
	})
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[{"id":1,"name":"default","ratio":1},{"id":2,"name":"unpublished","ratio":2}]`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"task-model":0.01}`))
	require.NoError(t, db.Create(&model.Supplier{ID: 1, Code: "preview", Name: "Preview", Adapter: "manual", Enabled: true}).Error)
	require.NoError(t, db.Create(&model.SupplierPricingProfile{ID: 1, SupplierID: 1, Code: "default", Enabled: true, USDPerCredit: 1, ConfigRevision: 1}).Error)
	require.NoError(t, db.Create(&model.PricingGroupSupplierBinding{GroupID: 2, SupplierID: 1, ProfileID: 1}).Error)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	info := &relaycommon.RelayInfo{UsingGroup: "auto", OriginModelName: "task-model"}

	require.NoError(t, previewTaskRoutes(ctx, info, []string{"2", "1"}))
	require.Len(t, info.AutoRoute.Candidates, 1)
	require.Equal(t, "1", info.AutoRoute.Candidates[0].Group)
	require.Nil(t, info.AutoRoute.Candidates[0].PriceData.SupplierTariff)
	require.NoError(t, orderQuotedAutoRoute(ctx, info, false))
	require.Equal(t, []string{"1"}, common.GetContextKeyStringSlice(ctx, constant.ContextKeyAutoGroupSnapshot))
	require.Equal(t, "1", info.UsingGroup)

	require.Error(t, previewTaskRoutes(ctx, info, []string{"2"}))
	require.Empty(t, info.AutoRoute.Candidates)
	require.Equal(t, "1", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
}

func TestAutoTaskQuoteValidatesContractsAfterFilteringSupplierRoutes(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	previousGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(previousGroups)) })
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"per-second","ratio":2},
		{"id":2,"name":"unpublished","ratio":2},
		{"id":3,"name":"token","ratio":2},
		{"id":4,"name":"expression","ratio":2},
		{"id":5,"name":"ambiguous","ratio":2}
	]`))
	require.NoError(t, db.Create(&model.Supplier{ID: 1, Code: "task-preview", Name: "Preview", Adapter: "manual", Enabled: true}).Error)
	unitPrice, tokenPrice := 0.1, 1.0
	tariffs := []types.SupplierModelTariff{
		{ModelName: "task-model", BillingMode: types.SupplierBillingModeFixed, PriceUnit: "per_second", PriceUSD: &unitPrice},
		{},
		{ModelName: "task-model", BillingMode: types.SupplierBillingModeToken, PriceUnit: "million_tokens", InputUSD: &tokenPrice, OutputUSD: &tokenPrice},
		{ModelName: "task-model", BillingMode: types.SupplierBillingModeExpression, PriceUnit: "million_tokens", Expression: "p * 1 + c * 2"},
	}
	for index, tariff := range tariffs {
		profileID := index + 1
		require.NoError(t, db.Create(&model.SupplierPricingProfile{ID: profileID, SupplierID: 1, Code: string(rune('a' + index)), Enabled: true, USDPerCredit: 1, ConfigRevision: 1}).Error)
		require.NoError(t, db.Create(&model.PricingGroupSupplierBinding{GroupID: profileID, SupplierID: 1, ProfileID: profileID}).Error)
		if index == 1 {
			continue
		}
		data, err := common.Marshal(tariff)
		require.NoError(t, err)
		require.NoError(t, db.Create(&model.SupplierPricingState{ProfileID: profileID, ActiveRevision: 1}).Error)
		require.NoError(t, db.Create(&model.SupplierPricingRevision{ProfileID: profileID, Revision: 1, ConfigRevision: 1}).Error)
		require.NoError(t, db.Create(&model.SupplierModelPrice{ProfileID: profileID, Revision: 1, ModelName: tariff.ModelName, Data: string(data)}).Error)
	}
	for index := range 6 {
		channel := model.Channel{Id: index + 1, Type: constant.ChannelTypeGemini, Status: common.ChannelStatusEnabled, Key: "task-preview-fixture-key"}
		group := index + 1
		if index == 0 {
			channel.Type = constant.ChannelTypeOpenAI
		}
		if group <= 4 {
			supplierID, profileID := 1, group
			channel.SupplierID, channel.SupplierProfileID = &supplierID, &profileID
		} else {
			group = 5
			mapping := []string{`{"task-model":"first-target"}`, `{"task-model":"second-target"}`}[index-4]
			channel.ModelMapping = &mapping
		}
		require.NoError(t, db.Create(&channel).Error)
		require.NoError(t, db.Create(&model.Ability{Group: strconv.Itoa(group), Model: "task-model", ChannelId: channel.Id, Enabled: true}).Error)
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	info := &relaycommon.RelayInfo{
		TokenGroup: "auto", UsingGroup: "auto", OriginModelName: "task-model", RequestedModelName: "task-model",
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}

	groups, err := resolveGroupBillingModels(ctx, info, []string{"2", "3", "4", "5", "1"})
	require.NoError(t, err)
	require.NotContains(t, groups, "5")
	require.NoError(t, previewTaskRoutes(ctx, info, groups))
	require.NoError(t, orderQuotedAutoRoute(ctx, info, false))
	require.Equal(t, []string{"1"}, common.GetContextKeyStringSlice(ctx, constant.ContextKeyAutoGroupSnapshot))
	require.Equal(t, "per_second", info.PriceData.SupplierTariff.PriceUnit)
	require.Empty(t, info.PriceData.OtherRatios, "the preliminary quote must not require a parsed duration")
	require.False(t, info.AttemptDispatched)

	// Exercise the real controller through preliminary quoting. Invalid task
	// input stops at adapter validation before any upstream call or debit.
	recorder := httptest.NewRecorder()
	requestContext, _ := gin.CreateTestContext(recorder)
	requestContext.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{}`))
	requestContext.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(requestContext, constant.ContextKeyTokenGroup, "auto")
	common.SetContextKey(requestContext, constant.ContextKeyUsingGroup, "auto")
	common.SetContextKey(requestContext, constant.ContextKeyOriginalModel, "task-model")
	common.SetContextKey(requestContext, constant.ContextKeyRequestedModel, "task-model")
	common.SetContextKey(requestContext, constant.ContextKeyAutoGroupSnapshot, []string{"2", "3", "4", "5", "1"})

	RelayTask(requestContext)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Equal(t, 1, common.GetContextKeyInt(requestContext, constant.ContextKeyChannelId))
	require.Equal(t, []string{"1"}, common.GetContextKeyStringSlice(requestContext, constant.ContextKeyAutoGroupSnapshot))
	require.Contains(t, recorder.Body.String(), `"code":"request_failed"`)
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

func TestQuotedRouteRejectsTransportChangedSinceSelection(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	baseURL := "https://original.example"
	channel := model.Channel{Id: 42, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Key: "original-fixture-key", BaseURL: &baseURL}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "1", Model: "model", ChannelId: channel.Id, Enabled: true}).Error)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "1")
	require.Nil(t, middleware.SetupContextForSelectedChannel(ctx, &channel, "model"))
	fingerprint := ctx.GetString("task_channel_transport_fingerprint")
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{"base_url": "https://changed.example", "key": "changed-fixture-key"}).Error)
	info := &relaycommon.RelayInfo{UsingGroup: "1", OriginModelName: "model", ForcePreConsume: true}

	require.Error(t, setupQuotedInitialChannel(ctx, info))
	require.Equal(t, fingerprint, ctx.GetString("task_channel_transport_fingerprint"))
	require.Equal(t, "original-fixture-key", common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
	require.Equal(t, baseURL, common.GetContextKeyString(ctx, constant.ContextKeyChannelBaseUrl))
	require.False(t, info.AttemptDispatched)
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
