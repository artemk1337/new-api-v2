package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDistributeRejectsForcedChannelFromAnotherPricingScope(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Supplier{}, &model.SupplierPricingProfile{}, &model.PricingGroupSupplierBinding{}))
	require.NoError(t, db.Create(&model.Supplier{ID: 1, Code: "private-supplier", Name: "Secret procurement source", Enabled: true}).Error)
	require.NoError(t, db.Create(&model.SupplierPricingProfile{ID: 1, SupplierID: 1, Code: "private-profile", Enabled: true}).Error)
	supplierID, profileID := 1, 1
	require.NoError(t, db.Create(&model.Channel{
		Id: 42, Key: "upstream-secret", Status: common.ChannelStatusEnabled,
		SupplierID: &supplierID, SupplierProfileID: &profileID,
	}).Error)
	dispatched := false
	router := gin.New()
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "1")
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "1")
		common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, "42")
	}, Distribute(), func(c *gin.Context) {
		dispatched = true
		c.Status(http.StatusNoContent)
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"model"}`))
	request.Header.Set("Content-Type", gin.MIMEJSON)

	router.ServeHTTP(recorder, request)

	require.False(t, dispatched)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "Secret procurement source")
	require.NotContains(t, recorder.Body.String(), "upstream-secret")
}
