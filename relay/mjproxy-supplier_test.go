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
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupMidjourneySupplierTest(t *testing.T) (*model.Channel, *model.Channel, *atomic.Int32) {
	t.Helper()
	previousDB := model.DB
	previousGroups := ratio_setting.PricingGroups2JSONString()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "midjourney.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Supplier{}, &model.SupplierPricingProfile{}, &model.PricingGroupSupplierBinding{}, &model.SupplierBindingState{}, &model.Midjourney{}))
	model.DB = db
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[{"id":1,"name":"default","ratio":1,"selectable":true},{"id":2,"name":"supplier-group","ratio":2,"selectable":true}]`))
	t.Cleanup(func() {
		model.DB = previousDB
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(previousGroups))
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})

	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)
	supplier := model.Supplier{Code: "supplier", Name: "Private Supplier", Adapter: "newapi", Enabled: true}
	require.NoError(t, db.Create(&supplier).Error)
	profile := model.SupplierPricingProfile{SupplierID: supplier.ID, Code: "profile", Name: "Private Profile", Enabled: true, USDPerCredit: 1}
	require.NoError(t, db.Create(&profile).Error)
	require.NoError(t, db.Create(&model.PricingGroupSupplierBinding{GroupID: 2, SupplierID: supplier.ID, ProfileID: profile.ID}).Error)
	bound := &model.Channel{Type: constant.ChannelTypeMidjourney, Key: "private-key", Name: "Bound", Status: common.ChannelStatusEnabled, Group: "2", BaseURL: &upstream.URL, SupplierID: &supplier.ID, SupplierProfileID: &profile.ID}
	legacy := &model.Channel{Type: constant.ChannelTypeMidjourney, Key: "legacy-key", Name: "Legacy", Status: common.ChannelStatusEnabled, Group: "1", BaseURL: &upstream.URL}
	require.NoError(t, db.Create(bound).Error)
	require.NoError(t, db.Create(legacy).Error)
	require.NoError(t, db.Create(&model.Midjourney{UserId: 1, MjId: "origin", Status: "SUCCESS", ChannelId: bound.Id}).Error)
	return bound, legacy, &requests
}

func midjourneySupplierContext(channel *model.Channel, group, body string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/mj/submit/imagine", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyChannelId, channel.Id)
	common.SetContextKey(c, constant.ContextKeyChannelType, channel.Type)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, channel.GetBaseURL())
	common.SetContextKey(c, constant.ContextKeyUsingGroup, group)
	c.Set("id", 1)
	return c
}

func TestMidjourneySupplierSubmissionDoesNotDispatch(t *testing.T) {
	bound, _, requests := setupMidjourneySupplierTest(t)

	c := midjourneySupplierContext(bound, "2", `{"prompt":"test"}`)
	response := RelayMidjourneySubmit(c, &relaycommon.RelayInfo{UserId: 1, UsingGroup: "2", RelayMode: relayconstant.RelayModeMidjourneyImagine})
	require.NotNil(t, response)
	require.Equal(t, "model_pricing_unavailable_for_operation", response.Description)
	require.Zero(t, requests.Load())

	c = midjourneySupplierContext(bound, "2", `{"sourceBase64":"source","targetBase64":"target"}`)
	response = RelaySwapFace(c, &relaycommon.RelayInfo{UserId: 1, UsingGroup: "2"})
	require.NotNil(t, response)
	require.Equal(t, "model_pricing_unavailable_for_operation", response.Description)
	require.Zero(t, requests.Load())
}

func TestMidjourneyOriginChannelCannotEscapePricingGroup(t *testing.T) {
	_, legacy, requests := setupMidjourneySupplierTest(t)
	c := midjourneySupplierContext(legacy, "1", `{"taskId":"origin","action":"UPSCALE","index":1}`)
	c.Request.Header.Set("Authorization", "Bearer user-token")
	response := RelayMidjourneySubmit(c, &relaycommon.RelayInfo{UserId: 1, UsingGroup: "1", RelayMode: relayconstant.RelayModeMidjourneyChange})
	require.NotNil(t, response)
	require.Equal(t, "model_pricing_unavailable_for_operation", response.Description)
	require.Equal(t, legacy.Id, c.GetInt("channel_id"))
	require.Equal(t, "Bearer user-token", c.GetHeader("Authorization"))
	require.Zero(t, requests.Load())

	c = midjourneySupplierContext(legacy, "1", "")
	c.Params = gin.Params{{Key: "id", Value: "origin"}}
	response = RelayMidjourneyTaskImageSeed(c)
	require.NotNil(t, response)
	require.Equal(t, "model_pricing_unavailable_for_operation", response.Description)
	require.Equal(t, legacy.Id, c.GetInt("channel_id"))
	require.Zero(t, requests.Load())
}

func TestMidjourneyLegacyPricingGroupRemainsAvailable(t *testing.T) {
	_, legacy, _ := setupMidjourneySupplierTest(t)
	c := midjourneySupplierContext(legacy, "1", "")
	require.Nil(t, validateMidjourneyChannelPricing(c, legacy))
	common.SetContextKey(c, constant.ContextKeyAutoGroup, "2")
	require.NotNil(t, validateMidjourneyChannelPricing(c, legacy))
}
