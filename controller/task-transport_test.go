package controller

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestVideoProxyDoesNotSendStoredCredentialsAfterTransportRace(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "task-transport.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Task{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB; sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests.Add(1); w.WriteHeader(http.StatusOK) }))
	defer upstream.Close()
	originalURL := upstream.URL + "/original"
	channel := &model.Channel{Type: constant.ChannelTypeSora, BaseURL: &originalURL, Key: "private-original-key"}
	require.NoError(t, db.Create(channel).Error)
	fingerprint := channel.TaskTransportFingerprint()
	require.NoError(t, db.Model(channel).Update("base_url", upstream.URL+"/replacement").Error)
	task := &model.Task{TaskID: "task_proxy_race", UserId: 1, ChannelId: channel.Id, Status: model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "private-original-id", Key: "private-original-key", TransportFingerprint: fingerprint}}
	require.NoError(t, db.Create(task).Error)
	engine := gin.New()
	engine.GET("/v1/videos/:task_id/content", func(c *gin.Context) { c.Set("id", 1); VideoProxy(c) })
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/videos/task_proxy_race/content", nil))
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Zero(t, requests.Load())
	require.NotContains(t, recorder.Body.String(), "private-original")
	require.NotContains(t, recorder.Body.String(), upstream.URL)
	require.NotContains(t, recorder.Body.String(), fingerprint)
}
