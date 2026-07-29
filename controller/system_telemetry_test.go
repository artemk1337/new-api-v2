package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetSystemTelemetryAgentUsesConfiguredSidecarURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/telemetry-agent", r.URL.Path)
		_, err := w.Write([]byte(`{"running":true,"message":""}`))
		require.NoError(t, err)
	}))
	defer server.Close()
	t.Setenv("UPDATE_SIDECAR_URL", server.URL+"/")

	previousClient := systemTelemetryHTTPClient
	systemTelemetryHTTPClient = server.Client()
	t.Cleanup(func() { systemTelemetryHTTPClient = previousClient })

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/system-info/telemetry-agent", nil)

	GetSystemTelemetryAgent(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"running":true,"message":""}`, recorder.Body.String())
}

func TestGetSystemTelemetryNormalizesMissingTopProcesses(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.SystemTelemetrySample{}))
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	require.NoError(t, model.CreateSystemTelemetrySample(&model.SystemTelemetrySample{
		NodeName:     "node-a",
		CollectedAt:  time.Now().Unix(),
		TopProcesses: "null",
	}))

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/system-info/telemetry?node_name=node-a", nil)

	GetSystemTelemetry(context)

	response := struct {
		Success bool                      `json:"success"`
		Data    []systemTelemetryResponse `json:"data"`
	}{}
	require.Equal(t, http.StatusOK, recorder.Code)
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Len(t, response.Data, 1)
	require.NotNil(t, response.Data[0].Top)
	require.Empty(t, response.Data[0].Top)
}
