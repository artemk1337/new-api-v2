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
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTaskAutoRequotesUnavailableSelectedSupplierBeforeDispatch(t *testing.T) {
	for _, kind := range []string{"unpublished", "token", "expression", "missing-duration"} {
		t.Run(kind, func(t *testing.T) {
			previousDB := model.DB
			previousGroups := ratio_setting.PricingGroups2JSONString()
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "auto.db")), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&model.Supplier{}, &model.SupplierPricingProfile{}, &model.PricingGroupSupplierBinding{}, &model.SupplierBindingState{}, &model.SupplierPricingState{}, &model.SupplierPricingRevision{}, &model.SupplierModelPrice{}))
			model.DB = db
			t.Cleanup(func() {
				model.DB = previousDB
				require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(previousGroups))
				connection, _ := db.DB()
				_ = connection.Close()
			})
			require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[{"id":1,"name":"default","ratio":1,"selectable":true},{"id":2,"name":"healthy","ratio":1,"selectable":true}]`))
			supplier := model.Supplier{Code: "test", Name: "Test", Adapter: "manual", Enabled: true}
			require.NoError(t, db.Create(&supplier).Error)
			bad := model.SupplierPricingProfile{SupplierID: supplier.ID, Code: "bad", Name: "Bad", Enabled: true, USDPerCredit: 1, ConfigRevision: 1}
			good := model.SupplierPricingProfile{SupplierID: supplier.ID, Code: "good", Name: "Good", Enabled: true, USDPerCredit: 1, ConfigRevision: 1}
			require.NoError(t, db.Create(&bad).Error)
			require.NoError(t, db.Create(&good).Error)
			require.NoError(t, db.Create(&[]model.PricingGroupSupplierBinding{{GroupID: 1, SupplierID: supplier.ID, ProfileID: bad.ID}, {GroupID: 2, SupplierID: supplier.ID, ProfileID: good.ID}}).Error)
			price := 0.0
			goodTariff := types.SupplierModelTariff{ModelName: "suno_music", BillingMode: types.SupplierBillingModeFixed, PriceUnit: "request", PriceUSD: &price}
			_, err = model.PublishSupplierProfileTariffs(good.ID, 1, 0, []types.SupplierModelTariff{goodTariff})
			require.NoError(t, err)
			if kind != "unpublished" {
				tariff := goodTariff.Clone()
				switch kind {
				case "token":
					tariff.BillingMode = types.SupplierBillingModeToken
					tariff.PriceUSD = nil
					tariff.PriceUnit = "million_tokens"
					tariff.InputUSD = &price
					tariff.OutputUSD = &price
				case "expression":
					tariff.BillingMode = types.SupplierBillingModeExpression
					tariff.PriceUSD = nil
					tariff.PriceUnit = "million_tokens"
					tariff.Expression = "p+c"
				case "missing-duration":
					tariff.PriceUnit = "per_second"
				}
				_, err = model.PublishSupplierProfileTariffs(bad.ID, 1, 0, []types.SupplierModelTariff{tariff})
				require.NoError(t, err)
			}
			var requests atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				require.Equal(t, "Bearer healthy-key", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"code":"success","data":"healthy-upstream-id"}`))
			}))
			defer upstream.Close()
			service.InitHttpClient()
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/suno/submit/MUSIC", strings.NewReader(`{"prompt":"a song"}`))
			ctx.Request.Header.Set("Content-Type", "application/json")
			ctx.Params = gin.Params{{Key: "action", Value: constant.SunoActionMusic}}
			ctx.Set("platform", string(constant.TaskPlatformSuno))
			ctx.Set("supplier_pricing", true)
			ctx.Set("task_channel_transport_fingerprint", "frozen")
			common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeSunoAPI)
			common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, "https://unavailable.example")
			common.SetContextKey(ctx, constant.ContextKeyChannelKey, "wrong-key")
			common.SetContextKey(ctx, constant.ContextKeyAutoGroup, "1")
			common.SetContextKey(ctx, constant.ContextKeyAutoGroupSnapshot, []string{"1", "2"})
			info := &relaycommon.RelayInfo{OriginModelName: "suno_music", RequestedModelName: "suno_music", UsingGroup: "1", TokenGroup: "auto", GroupBillingModelNames: map[string]string{"1": "suno_music", "2": "suno_music"}, TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_auto"}}
			result, taskError := RelayTaskSubmit(ctx, info)
			require.Nil(t, result)
			require.NotNil(t, taskError)
			require.Equal(t, "auto_route_requote", taskError.Code)
			require.False(t, info.AttemptDispatched)
			require.Nil(t, info.Billing)
			require.Zero(t, requests.Load())
			require.Len(t, info.AutoRoute.Candidates, 1)
			require.Equal(t, "2", info.AutoRoute.InitialGroup)
			common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, upstream.URL)
			common.SetContextKey(ctx, constant.ContextKeyChannelKey, "healthy-key")
			common.SetContextKey(ctx, constant.ContextKeyAutoGroup, "2")
			info.ResetAttempt()
			result, taskError = RelayTaskSubmit(ctx, info)
			require.Nil(t, taskError)
			require.NotNil(t, result)
			require.Equal(t, "healthy-upstream-id", result.UpstreamTaskID)
			require.EqualValues(t, 1, requests.Load())
			require.Equal(t, good.ID, info.PriceData.SupplierTariff.ProfileID)
		})
	}
}
