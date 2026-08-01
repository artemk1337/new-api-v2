package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestFetchUpstreamRatiosReturnsStatesForDifferences(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:pricing-sync-fetch-states?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.PricingSyncModelState{}))
	require.NoError(t, db.Create(&model.PricingSyncModelState{
		ModelName: "manual-model",
		Mode:      model.PricingSyncModelModeManual,
		Status:    model.PricingSyncModelStatusReady,
	}).Error)
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"model_ratio":{"manual-model":2,"new-model":3}}}`))
	}))
	t.Cleanup(upstream.Close)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/ratio_sync/fetch", bytes.NewBufferString(`{
		"upstreams":[{"name":"upstream","base_url":"`+upstream.URL+`"}]
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	FetchUpstreamRatios(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Differences   map[string]map[string]dto.DifferenceItem `json:"differences"`
			ModelStates   map[string]model.PricingSyncModelState   `json:"model_states"`
			ConfigVersion int64                                    `json:"config_version"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Zero(t, response.Data.ConfigVersion)
	require.Contains(t, response.Data.Differences, "manual-model")
	require.Contains(t, response.Data.Differences, "new-model")
	require.Equal(t, map[string]model.PricingSyncModelState{
		"manual-model": {
			ModelName: "manual-model",
			Mode:      model.PricingSyncModelModeManual,
			Status:    model.PricingSyncModelStatusReady,
		},
		"new-model": {
			ModelName: "new-model",
			Mode:      model.PricingSyncModelModeGeneral,
			ChannelID: 0,
			Status:    model.PricingSyncModelStatusReady,
		},
	}, response.Data.ModelStates)
}
