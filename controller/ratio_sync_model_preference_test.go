package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetPricingSyncModelPreferenceAcceptsSlashInQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:pricing-sync-model-preference?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PricingSyncModelState{}, &model.PricingSyncSource{}))

	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	modelName := "anthropic/claude-sonnet"
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/ratio_sync/model-preference?model="+url.QueryEscape(modelName),
		nil,
	)

	GetPricingSyncModelPreference(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"model_name":"anthropic/claude-sonnet"`)
}

func TestUpdatePricingSyncConfigRequiresExpectedVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/ratio_sync/config", bytes.NewReader([]byte(`{
		"strategy":"highest","sources":[]
	}`)))
	ctx.Request.Header.Set("Content-Type", "application/json")

	UpdatePricingSyncConfig(ctx)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "version is required")
}

func TestUpdatePricingSyncModelPreferencePreservesPricingProvenance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:pricing-sync-model-preference-update?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.PricingSyncModelState{}, &model.PricingSyncSource{}))
	require.NoError(t, db.Create(&model.PricingSyncModelState{
		ModelName: "model-a", Mode: model.PricingSyncModelModeGeneral,
		Provenance: "[8,9]", ConflictDetails: `{"8":{"model_price":1}}`,
		Status: model.PricingSyncModelStatusConflict, LastAppliedAt: 42,
	}).Error)
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/ratio_sync/model-preference", bytes.NewReader([]byte(`{
		"model_name":"model-a","mode":"channel","channel_id":8
	}`)))
	ctx.Request.Header.Set("Content-Type", "application/json")
	require.NoError(t, db.Create(&model.PricingSyncSource{ChannelID: 8, Enabled: true}).Error)

	UpdatePricingSyncModelPreference(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	state, err := model.GetPricingSyncModelState("model-a")
	require.NoError(t, err)
	require.Equal(t, model.PricingSyncModelModeChannel, state.Mode)
	require.Equal(t, 8, state.ChannelID)
	require.Equal(t, model.PricingSyncModelStatusStale, state.Status)
	require.Equal(t, "[8,9]", state.Provenance)
	require.Equal(t, int64(42), state.LastAppliedAt)
}

func TestUpdatePricingSyncModelPreferenceWithoutStateIsStaleAndNormalizesChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:pricing-sync-model-preference-new?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.PricingSyncModelState{}))
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/ratio_sync/model-preference", bytes.NewReader([]byte(`{
		"model_name":"model-new","mode":"general","channel_id":99
	}`)))
	ctx.Request.Header.Set("Content-Type", "application/json")

	UpdatePricingSyncModelPreference(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	state, err := model.GetPricingSyncModelState("model-new")
	require.NoError(t, err)
	require.Equal(t, model.PricingSyncModelStatusStale, state.Status)
	require.Zero(t, state.ChannelID)
}

func TestApplyPricingSyncPatchesPersistsPreferencesAtomically(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:pricing-sync-apply-preferences?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Option{},
		&model.PricingSyncSource{},
		&model.PricingSyncModelState{},
	))
	require.NoError(t, db.Create(&model.PricingSyncSource{
		ChannelID: 8, Enabled: true, Endpoint: "/api/pricing",
	}).Error)
	require.NoError(t, db.Create([]model.PricingSyncModelState{
		{ModelName: "model-a", Mode: model.PricingSyncModelModeGeneral, Provenance: "[7,9]", Status: model.PricingSyncModelStatusConflict, LastAppliedAt: 50},
		{ModelName: "model-b", Mode: model.PricingSyncModelModeGeneral, Provenance: "[7]", Status: model.PricingSyncModelStatusReady, LastAppliedAt: 50},
	}).Error)
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	body := []byte(`{
		"patches":{"ModelPrice":{"set":{"channel-model":1,"general-model":2,"manual-model":3}}},
		"preferences":[
			{"model_name":"channel-model","mode":"channel","channel_id":8},
			{"model_name":"general-model","mode":"general","channel_id":999}
		]
	}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/ratio_sync/apply", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ApplyPricingSyncPatches(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	channelState, err := model.GetPricingSyncModelState("channel-model")
	require.NoError(t, err)
	require.Equal(t, model.PricingSyncModelModeChannel, channelState.Mode)
	require.Equal(t, 8, channelState.ChannelID)
	generalState, err := model.GetPricingSyncModelState("general-model")
	require.NoError(t, err)
	require.Equal(t, model.PricingSyncModelModeGeneral, generalState.Mode)
	require.Zero(t, generalState.ChannelID)
	manualState, err := model.GetPricingSyncModelState("manual-model")
	require.NoError(t, err)
	require.Equal(t, model.PricingSyncModelModeManual, manualState.Mode)
}

func TestApplyPricingSyncPatchesRejectsDisabledSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:pricing-sync-apply-disabled?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Option{},
		&model.PricingSyncSource{},
		&model.PricingSyncModelState{},
	))
	require.NoError(t, db.Create(&model.PricingSyncSource{
		ChannelID: 8, Enabled: false, Endpoint: "/api/pricing",
	}).Error)

	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	body := []byte(`{
		"patches":{"ModelPrice":{"set":{"model-a":1}}},
		"preferences":[{"model_name":"model-a","mode":"channel","channel_id":8}]
	}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/ratio_sync/apply", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ApplyPricingSyncPatches(ctx)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	var count int64
	require.NoError(t, db.Model(&model.Option{}).Where("key = ?", "ModelPrice").Count(&count).Error)
	require.Zero(t, count)
}

func TestApplyPricingSyncPatchesAcceptsPreferenceWithoutPatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:pricing-sync-apply-preference-only?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Option{},
		&model.PricingSyncSource{},
		&model.PricingSyncModelState{},
	))
	require.NoError(t, db.Create(&model.PricingSyncSource{
		ChannelID: 8, Enabled: true, Endpoint: "/api/pricing",
	}).Error)
	require.NoError(t, db.Create([]model.PricingSyncModelState{
		{
			ModelName: "model-a", Mode: model.PricingSyncModelModeGeneral,
			Provenance: "[7,9]", Status: model.PricingSyncModelStatusConflict,
			ConflictDetails: `{"7":{"model_price":1}}`, LastAppliedAt: 50,
		},
		{
			ModelName: "model-b", Mode: model.PricingSyncModelModeGeneral,
			Provenance: "[7]", Status: model.PricingSyncModelStatusReady,
			LastAppliedAt: 40,
		},
	}).Error)

	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	body := []byte(`{
		"expected_version":0,
		"preferences":[
			{"model_name":"model-a","mode":"channel","channel_id":8},
			{"model_name":"model-b","mode":"general","channel_id":0}
		]
	}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/ratio_sync/apply", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ApplyPricingSyncPatches(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var response struct {
		Data struct {
			ConfigVersion int64 `json:"config_version"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, int64(1), response.Data.ConfigVersion)
	state, err := model.GetPricingSyncModelState("model-a")
	require.NoError(t, err)
	require.Equal(t, model.PricingSyncModelModeChannel, state.Mode)
	require.Equal(t, 8, state.ChannelID)
	require.Equal(t, model.PricingSyncModelStatusStale, state.Status)
	require.Equal(t, "[7,9]", state.Provenance)
	require.Equal(t, int64(50), state.LastAppliedAt)
	generalState, err := model.GetPricingSyncModelState("model-b")
	require.NoError(t, err)
	require.Equal(t, model.PricingSyncModelModeGeneral, generalState.Mode)
	require.Equal(t, model.PricingSyncModelStatusStale, generalState.Status)
	require.Equal(t, "[7]", generalState.Provenance)
}

func TestApplyPricingSyncPatchesRejectsStaleModelStateSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:pricing-sync-apply-stale-snapshot?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Option{},
		&model.PricingSyncSource{},
		&model.PricingSyncModelState{},
	))
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	version, err := model.GetPricingSyncConfigVersion()
	require.NoError(t, err)
	require.Zero(t, version)
	require.NoError(t, model.ApplyPricingSyncUpdateWithPreferences(nil, []model.PricingSyncModelPreferenceInput{{
		ModelName: "model-a",
		Mode:      model.PricingSyncModelModeManual,
	}}))

	body := []byte(`{
		"patches":{"ModelPrice":{"set":{"model-a":1}}},
		"preferences":[{"model_name":"model-a","mode":"general","channel_id":0}],
		"expected_version":0
	}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/ratio_sync/apply", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ApplyPricingSyncPatches(ctx)

	require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
	state, err := model.GetPricingSyncModelState("model-a")
	require.NoError(t, err)
	require.Equal(t, model.PricingSyncModelModeManual, state.Mode)
	var count int64
	require.NoError(t, db.Model(&model.Option{}).Where("key = ?", "ModelPrice").Count(&count).Error)
	require.Zero(t, count)
}
