package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupLegacySupplierControllerDB(t *testing.T) *gorm.DB {
	t.Helper()
	original := model.DB
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.SupplierBindingState{}, &model.Option{}, &model.PricingSyncSource{}, &model.PricingSyncQuote{}, &model.PricingSyncModelState{}))
	model.DB = db
	t.Cleanup(func() { model.DB = original; sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	supplierID, profileID := 1, 2
	base := "https://supplier.example"
	require.NoError(t, db.Create(&model.Channel{Id: 1, Name: "supplier", SupplierID: &supplierID, SupplierProfileID: &profileID, BaseURL: &base, Status: common.ChannelStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 2, Name: "legacy", BaseURL: &base, Status: common.ChannelStatusEnabled}).Error)
	return db
}

func TestLegacyPreviewRejectsSupplierChannelBeforeFetch(t *testing.T) {
	setupLegacySupplierControllerDB(t)
	for _, body := range []string{`{"channel_ids":[1]}`, `{"upstreams":[{"id":1,"name":"supplied","base_url":"https://supplier.example"}]}`} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		ctx.Request.Header.Set("Content-Type", "application/json")
		FetchUpstreamRatios(ctx)
		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Contains(t, recorder.Body.String(), model.ErrPricingSyncSupplierBound.Error())
	}
}

func TestLegacyChannelInventoryExcludesSupplierAndKeepsPresets(t *testing.T) {
	setupLegacySupplierControllerDB(t)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	GetSyncableChannels(ctx)
	var body struct {
		Success bool                  `json:"success"`
		Data    []dto.SyncableChannel `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	ids := make([]int, 0, len(body.Data))
	for _, channel := range body.Data {
		ids = append(ids, channel.ID)
	}
	require.NotContains(t, ids, 1)
	require.Contains(t, ids, 2)
	require.Contains(t, ids, officialRatioPresetID)
	require.Contains(t, ids, modelsDevPresetID)
}

func TestLegacySchedulerDoesNotFetchOrMutateSupplierSource(t *testing.T) {
	db := setupLegacySupplierControllerDB(t)
	source := model.PricingSyncSource{ChannelID: 1, Enabled: true, Endpoint: "/api/pricing", IntervalSeconds: 60, LastAttemptAt: 99, LastError: "keep-old-error"}
	require.NoError(t, db.Create(&source).Error)
	require.NoError(t, db.Create(&model.Option{Key: "ModelRatio", Value: `{"keep-model":2}`}).Error)
	require.NoError(t, db.Create(&model.PricingSyncQuote{ChannelID: 1, ModelName: "keep-model", Data: `{"model_ratio":99}`, Category: "ratio", MissingCount: 3, Confirmations: 3}).Error)
	summary, err := runUpstreamPricingSyncTaskOnce(context.Background(), "", nil)
	require.NoError(t, err)
	require.Zero(t, summary.CheckedChannels)
	require.Zero(t, summary.AppliedModels)
	require.Zero(t, summary.DeletedPricingItems)
	var current model.PricingSyncSource
	require.NoError(t, db.First(&current, "channel_id = ?", 1).Error)
	require.Equal(t, source, current)
	var option model.Option
	require.NoError(t, db.First(&option, "key = ?", "ModelRatio").Error)
	require.Equal(t, `{"keep-model":2}`, option.Value)
	channel, err := model.GetChannelById(1, true)
	require.NoError(t, err)
	_, err = pricingSyncFetchTargetForSource(source, channel)
	require.ErrorIs(t, err, model.ErrPricingSyncSupplierBound)
	_, err = fetchChannelPricing(context.Background(), channel, map[string]struct{}{"supplier.example": {}})
	require.ErrorIs(t, err, model.ErrPricingSyncSupplierBound)
	for _, id := range []int{officialRatioPresetID, modelsDevPresetID} {
		_, err := pricingSyncFetchTargetForSource(model.PricingSyncSource{ChannelID: id}, nil)
		require.NoError(t, err)
	}
}

func TestLegacyCustomURLPreviewRemainsDefaultScope(t *testing.T) {
	setupLegacySupplierControllerDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"model_ratio":{"model-a":1},"completion_ratio":{"model-a":2}}}`))
	}))
	defer server.Close()
	body, err := common.Marshal(dto.UpstreamRequest{Upstreams: []dto.UpstreamDTO{{Name: "manual-default-source", BaseURL: server.URL}}})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	ctx.Request.Header.Set("Content-Type", "application/json")
	FetchUpstreamRatios(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	var result struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
	require.True(t, result.Success, recorder.Body.String())
}

func TestLegacyConfigurationHidesDormantSupplierSources(t *testing.T) {
	db := setupLegacySupplierControllerDB(t)
	require.NoError(t, db.Create(&model.PricingSyncSource{ChannelID: 1, Enabled: true, Endpoint: "/api/pricing", IntervalSeconds: 60}).Error)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	GetPricingSyncConfig(ctx)
	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Sources []model.PricingSyncSource `json:"sources"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
	require.True(t, result.Success)
	require.Empty(t, result.Data.Sources)
	var source model.PricingSyncSource
	require.NoError(t, db.First(&source, "channel_id = ?", 1).Error)
	require.True(t, source.Enabled)
}
