package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func callLogFilterHandler(t *testing.T, handler gin.HandlerFunc, role int, values url.Values) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/log?"+values.Encode(), nil)
	ctx.Set("role", role)
	ctx.Set("id", 1)
	handler(ctx)
	return response
}

func TestLogSupplierFiltersAndSuggestionsRejectOrdinaryUsers(t *testing.T) {
	for _, role := range []int{0, common.RoleCommonUser} {
		for _, handler := range []gin.HandlerFunc{GetLogSuggestions, GetLogSupplierChoices} {
			response := callLogFilterHandler(t, handler, role, url.Values{"field": {"model_name"}})
			require.Equal(t, http.StatusForbidden, response.Code)
		}
		for _, handler := range []gin.HandlerFunc{GetUserLogs, GetLogsSelfStat, GetLogByKey, KeySessionLogs} {
			for _, value := range []string{"", "0", "1"} {
				response := callLogFilterHandler(t, handler, role, url.Values{"proxy_supplier_id": {value}})
				require.Equal(t, http.StatusForbidden, response.Code)
			}
		}
	}
}

func TestLogSuggestionHTTPPaginationAndInputValidation(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	for i := 0; i < 105; i++ {
		require.NoError(t, db.Create(&model.Log{ModelName: "model-" + strconv.Itoa(100+i), Type: model.LogTypeConsume}).Error)
	}
	response := callLogFilterHandler(t, GetLogSuggestions, common.RoleAdminUser, url.Values{"field": {"model_name"}})
	var payload struct {
		Success bool                    `json:"success"`
		Data    model.LogSuggestionPage `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Len(t, payload.Data.Items, 50)
	require.Equal(t, "50", *payload.Data.NextCursor)
	response = callLogFilterHandler(t, GetLogSuggestions, common.RoleAdminUser, url.Values{"field": {"model_name"}, "cursor": {"50"}, "limit": {"1000"}})
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	require.Len(t, payload.Data.Items, 55)
	require.Nil(t, payload.Data.NextCursor)
	response = callLogFilterHandler(t, GetLogSuggestions, common.RoleAdminUser, url.Values{"field": {"model_name"}, "limit": {"1000"}})
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	require.Len(t, payload.Data.Items, 100)
	for _, values := range []url.Values{
		{"field": {"supplier_id"}}, {"field": {"other"}}, {"field": {"model_name"}, "cursor": {"-1"}},
		{"field": {"model_name"}, "cursor": {"999999999999999999999"}}, {"field": {"model_name"}, "limit": {"0"}},
		{"field": {"model_name"}, "proxy_supplier_id": {"-1"}}, {"field": {"model_name"}, "proxy_supplier_id": {""}},
	} {
		response = callLogFilterHandler(t, GetLogSuggestions, common.RoleAdminUser, values)
		require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	}
}

func TestLogSupplierHTTPFilterMatchesRowsCountAndStats(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	now := common.GetTimestamp()
	for _, row := range []model.Log{
		{Type: model.LogTypeConsume, ModelName: "model_under", Username: "alice", TokenName: "token%literal", Group: "7", ProxySupplierID: 11, Quota: 17, PromptTokens: 2, CompletionTokens: 3, CreatedAt: now, RequestId: "matching"},
		{Type: model.LogTypeConsume, ModelName: "model_under", Username: "alice", TokenName: "token%literal", Group: "7", ProxySupplierID: 12, Quota: 31, CreatedAt: now, RequestId: "other-supplier"},
		{Type: model.LogTypeConsume, ModelName: "model_under", Username: "alice", TokenName: "token%literal", Group: "7", Quota: 13, CreatedAt: now, RequestId: "unknown"},
		{Type: model.LogTypeError, ModelName: "model_under", Username: "alice", TokenName: "token%literal", Group: "7", ProxySupplierID: 11, Quota: 99, CreatedAt: now, RequestId: "error"},
	} {
		require.NoError(t, db.Create(&row).Error)
	}
	filters := url.Values{"type": {"2"}, "model_name": {"model_under"}, "username": {"alice"}, "token_name": {"token%literal"}, "group": {"7"}, "proxy_supplier_id": {"11"}, "start_timestamp": {strconv.FormatInt(now-10, 10)}, "end_timestamp": {strconv.FormatInt(now, 10)}, "request_id": {"matching"}, "page_size": {"1"}}
	response := callLogFilterHandler(t, GetAllLogs, common.RoleAdminUser, filters)
	var list struct {
		Success bool `json:"success"`
		Data    struct {
			Total int         `json:"total"`
			Items []model.Log `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &list))
	require.True(t, list.Success)
	require.Equal(t, 1, list.Data.Total)
	require.Len(t, list.Data.Items, 1)
	require.Equal(t, "matching", list.Data.Items[0].RequestId)
	response = callLogFilterHandler(t, GetLogsStat, common.RoleAdminUser, filters)
	var stat struct {
		Success bool       `json:"success"`
		Data    model.Stat `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &stat))
	require.True(t, stat.Success)
	require.Equal(t, model.Stat{Quota: 17, Rpm: 1, Tpm: 5}, stat.Data)
	filters.Set("proxy_supplier_id", "0")
	filters.Del("request_id")
	response = callLogFilterHandler(t, GetAllLogs, common.RoleAdminUser, filters)
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &list))
	require.Equal(t, 1, list.Data.Total)
	require.Equal(t, "unknown", list.Data.Items[0].RequestId)
	response = callLogFilterHandler(t, GetLogsStat, common.RoleAdminUser, filters)
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &stat))
	require.Equal(t, 13, stat.Data.Quota)
}

func TestLogSupplierChoicesHTTPContainsOnlyRecordedIdentity(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	for _, supplier := range []model.Supplier{
		{ID: 11, Code: "private-code-a", Name: "Supplier A", Adapter: "private-adapter", Enabled: true},
		{ID: 12, Code: "private-code-b", Name: "Supplier B", Adapter: "private-adapter", Enabled: true},
		{ID: 13, Code: "private-unused", Name: "Unused supplier", Adapter: "private-adapter", Enabled: true},
	} {
		require.NoError(t, db.Create(&supplier).Error)
	}
	require.NoError(t, db.Create(&[]model.Log{
		{Type: model.LogTypeConsume, ModelName: "visible", ProxySupplierID: 11},
		{Type: model.LogTypeConsume, ModelName: "visible", ProxySupplierID: 12},
		{Type: model.LogTypeConsume, ModelName: "other-filter", ProxySupplierID: 14},
	}).Error)
	response := callLogFilterHandler(t, GetLogSupplierChoices, common.RoleAdminUser, url.Values{"model_name": {"visible"}, "proxy_supplier_id": {"11"}, "limit": {"1"}})
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"success":true,"message":"","data":{"items":[{"id":11,"name":"Supplier A"}],"next_cursor":"1"}}`, response.Body.String())
	response = callLogFilterHandler(t, GetLogSupplierChoices, common.RoleAdminUser, url.Values{"model_name": {"visible"}, "proxy_supplier_id": {"11"}, "limit": {"1"}, "cursor": {"1"}})
	require.JSONEq(t, `{"success":true,"message":"","data":{"items":[{"id":12,"name":"Supplier B"}],"next_cursor":null}}`, response.Body.String())
	for _, secret := range []string{"private-", "unused", "profile", "credential", "adapter", "usd"} {
		require.NotContains(t, response.Body.String(), secret)
	}
}

func TestLogSupplierErrorSnapshotSurvivesAdminDiagnostics(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	previous := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = true
	t.Cleanup(func() { constant.ErrorLogEnabled = previous })
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set("id", 1)
	ctx.Set("original_model", "public-model")
	ctx.Set("proxy_supplier_id", 22)
	info := &relaycommon.RelayInfo{PriceData: types.PriceData{SupplierTariff: &types.SupplierModelTariff{SupplierID: 11}}}
	processChannelError(ctx, types.ChannelError{ChannelId: 42}, types.NewErrorWithStatusCode(errors.New("upstream failure"), types.ErrorCodeBadResponse, http.StatusBadGateway), info)
	var stored model.Log
	require.NoError(t, db.First(&stored).Error)
	require.Equal(t, 11, stored.ProxySupplierID, "later admin diagnostics must preserve the frozen tariff identity")
}
