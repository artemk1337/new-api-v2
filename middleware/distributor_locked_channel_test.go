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
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSetupContextForLockedChannelPreservesSelectedKey(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyChannelKey, "origin-key")
	common.SetContextKey(ctx, constant.ContextKeyChannelIsMultiKey, true)
	common.SetContextKey(ctx, constant.ContextKeyChannelMultiKeyIndex, 2)
	mapping := `{"alias-model":"provider-model"}`
	channel := &model.Channel{Id: 42, Name: "locked", Key: "next-key", ModelMapping: &mapping}

	setupErr := SetupContextForLockedChannel(ctx, channel, "alias-model")
	require.Nil(t, setupErr, "%#v", setupErr)
	require.Equal(t, "origin-key", common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
	require.Equal(t, mapping, common.GetContextKeyString(ctx, constant.ContextKeyChannelModelMapping))
	require.Equal(t, 42, common.GetContextKeyInt(ctx, constant.ContextKeyChannelId))
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyChannelIsMultiKey))
	require.Equal(t, 2, common.GetContextKeyInt(ctx, constant.ContextKeyChannelMultiKeyIndex))
}

func TestDistributeAppliesKeyMappingBeforeSpecificChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB := model.DB
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, db.Create(&model.Channel{
		Id:     42,
		Name:   "locked",
		Key:    "upstream-key",
		Status: common.ChannelStatusEnabled,
	}).Error)

	router := gin.New()
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, "42")
		common.SetContextKey(c, constant.ContextKeyTokenModelMapping, `{"client-alias":"provider-model"}`)
		common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
		common.SetContextKey(c, constant.ContextKeyTokenModelLimit, map[string]bool{"client-alias": true})
	}, Distribute(), func(c *gin.Context) {
		assert.Equal(t, "provider-model", c.GetString("original_model"))
		assert.Equal(t, "client-alias", common.GetContextKeyString(c, constant.ContextKeyRequestedModel))
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"client-alias"}`))
	request.Header.Set("Content-Type", gin.MIMEJSON)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestDistributeAppliesKeyMappingBeforeSpecificChannelForCompactResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB := model.DB
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, db.Create(&model.Channel{
		Id:     42,
		Name:   "locked",
		Key:    "upstream-key",
		Status: common.ChannelStatusEnabled,
	}).Error)

	router := gin.New()
	router.POST("/v1/responses/compact", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, "42")
		common.SetContextKey(c, constant.ContextKeyTokenModelMapping, `{"client-alias":"provider-model"}`)
		common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
		common.SetContextKey(c, constant.ContextKeyTokenModelLimit, map[string]bool{ratio_setting.WithCompactModelSuffix("client-alias"): true})
	}, Distribute(), func(c *gin.Context) {
		assert.Equal(t, ratio_setting.WithCompactModelSuffix("provider-model"), c.GetString("original_model"))
		assert.Equal(t, ratio_setting.WithCompactModelSuffix("client-alias"), common.GetContextKeyString(c, constant.ContextKeyRequestedModel))
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses/compact", strings.NewReader(`{"model":"client-alias"}`))
	request.Header.Set("Content-Type", gin.MIMEJSON)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code)
}
