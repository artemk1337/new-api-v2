package relay

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTaskAutoNormalizesEachRoutePriceUnitBeforeDispatch(t *testing.T) {
	for _, tc := range []struct {
		name        string
		unit        string
		mixed       bool
		selected    string
		wantQuota   int
		wantReserve int
		wantCode    string
	}{
		{"global-per-call", "per_call", false, "1", 5000, 5000, string(types.ErrorCodeInsufficientAutoReserve)},
		{"global-per-second", "per_second", false, "1", 20000, 20000, string(types.ErrorCodeInsufficientAutoReserve)},
		{"mixed-global-selected", "per_call", true, "1", 5000, 20000, string(types.ErrorCodeInsufficientAutoReserve)},
		{"mixed-supplier-selected", "per_call", true, "2", 20000, 20000, "auto_route_requote"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previousDB, previousLogDB, previousRedis := model.DB, model.LOG_DB, common.RedisEnabled
			previousGroups, previousPrices := ratio_setting.PricingGroups2JSONString(), ratio_setting.ModelPrice2JSONString()
			previousUnits, err := common.Marshal(billing_setting.GetTaskPriceUnitCopy())
			require.NoError(t, err)
			previousQuotaPerUnit := common.GetQuotaPerUnit()
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "auto.db")), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Log{}, &model.Supplier{}, &model.SupplierPricingProfile{}, &model.PricingGroupSupplierBinding{}, &model.SupplierBindingState{}, &model.SupplierPricingState{}, &model.SupplierPricingRevision{}, &model.SupplierModelPrice{}))
			model.DB, model.LOG_DB, common.RedisEnabled = db, db, false
			common.SetQuotaPerUnit(500000)
			t.Cleanup(func() {
				model.DB, model.LOG_DB, common.RedisEnabled = previousDB, previousLogDB, previousRedis
				common.SetQuotaPerUnit(previousQuotaPerUnit)
				require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(previousGroups))
				require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousPrices))
				require.NoError(t, billing_setting.UpdateFromMap(map[string]string{billing_setting.TaskPriceUnitField: string(previousUnits)}))
				connection, _ := db.DB()
				_ = connection.Close()
			})
			require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[{"id":1,"name":"default","ratio":1,"selectable":true},{"id":2,"name":"supplier","ratio":1,"selectable":true}]`))
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"sora-2":0.01}`))
			require.NoError(t, billing_setting.UpdateFromMap(map[string]string{billing_setting.TaskPriceUnitField: `{"sora-2":"` + tc.unit + `"}`}))
			user := model.User{Id: 901, Username: "task-quote", Quota: 0}
			token := model.Token{Id: 901, UserId: user.Id, Key: "task-quote", RemainQuota: 100000}
			require.NoError(t, db.Create(&user).Error)
			require.NoError(t, db.Create(&token).Error)
			groups := []string{"1"}
			if tc.mixed {
				groups = []string{"1", "2"}
				supplier := model.Supplier{Code: "quote", Name: "Quote", Adapter: "manual", Enabled: true}
				require.NoError(t, db.Create(&supplier).Error)
				profile := model.SupplierPricingProfile{SupplierID: supplier.ID, Code: "seconds", Name: "Seconds", Enabled: true, USDPerCredit: 1, ConfigRevision: 1}
				require.NoError(t, db.Create(&profile).Error)
				require.NoError(t, db.Create(&model.PricingGroupSupplierBinding{GroupID: 2, SupplierID: supplier.ID, ProfileID: profile.ID}).Error)
				price := 0.01
				_, err = model.PublishSupplierProfileTariffs(profile.ID, 1, 0, []types.SupplierModelTariff{{ModelName: "sora-2", BillingMode: types.SupplierBillingModeFixed, PriceUnit: "per_second", PriceUSD: &price}})
				require.NoError(t, err)
			}
			var requests atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusTeapot)
			}))
			defer upstream.Close()
			service.InitHttpClient()
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"sora-2","prompt":"a cat","seconds":"4"}`))
			ctx.Request.Header.Set("Content-Type", "application/json")
			ctx.Set("task_channel_transport_fingerprint", "frozen")
			ctx.Set("supplier_pricing", tc.selected == "2")
			common.SetContextKey(ctx, constant.ContextKeyChannelId, 1)
			common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
			common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, upstream.URL)
			common.SetContextKey(ctx, constant.ContextKeyAutoGroup, tc.selected)
			common.SetContextKey(ctx, constant.ContextKeyAutoGroupSnapshot, groups)
			info := &relaycommon.RelayInfo{UserId: user.Id, TokenId: token.Id, OriginModelName: "sora-2", RequestedModelName: "sora-2", UsingGroup: tc.selected, TokenGroup: "auto", TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
			info.UserSetting.BillingPreference = "wallet_only"
			result, taskError := RelayTaskSubmit(ctx, info)
			require.Nil(t, result)
			require.NotNil(t, taskError)
			require.Equal(t, tc.wantCode, taskError.Code)
			require.Equal(t, tc.wantQuota, info.PriceData.Quota)
			require.Equal(t, tc.wantReserve, info.AutoRoute.ReservedQuota)
			global, exists := info.AutoRouteCandidate("1")
			require.True(t, exists)
			if tc.unit == billing_setting.TaskPriceUnitPerCall {
				require.Equal(t, 5000, global.EstimatedQuota)
				require.NotContains(t, global.PriceData.OtherRatios, "seconds")
			} else {
				require.Equal(t, 20000, global.EstimatedQuota)
				require.Equal(t, 4.0, global.PriceData.OtherRatios["seconds"])
			}
			if tc.mixed {
				supplier, exists := info.AutoRouteCandidate("2")
				require.True(t, exists)
				require.Equal(t, 20000, supplier.EstimatedQuota)
				require.Equal(t, 4.0, supplier.PriceData.OtherRatios["seconds"])
			}
			require.False(t, info.AttemptDispatched)
			require.Nil(t, info.Billing)
			require.Zero(t, requests.Load())
			require.NoError(t, db.First(&user, user.Id).Error)
			require.NoError(t, db.First(&token, token.Id).Error)
			require.Zero(t, user.Quota)
			require.Equal(t, 100000, token.RemainQuota)
		})
	}
}
