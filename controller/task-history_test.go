package controller

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTaskResultControllerDB(t *testing.T) *gorm.DB {
	t.Helper()
	previous := model.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "history.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Task{}))
	model.DB = db
	setting := system_setting.GetFetchSetting()
	saved := *setting
	*setting = system_setting.FetchSetting{EnableSSRFProtection: true, AllowPrivateIp: true, AllowedPorts: []string{"1-65535"}}
	service.InitHttpClient()
	t.Cleanup(func() { model.DB = previous; *setting = saved; connection, _ := db.DB(); _ = connection.Close() })
	return db
}

func TestCompletedVideoUsesOriginalTransportAfterChannelRotation(t *testing.T) {
	db := setupTaskResultControllerDB(t)
	var oldRequests, newRequests atomic.Int32
	original := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		oldRequests.Add(1)
		require.Equal(t, "/v1/videos/private-id/content", r.URL.Path)
		require.Equal(t, "Bearer original-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("original video"))
	}))
	defer original.Close()
	replacement := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { newRequests.Add(1); w.WriteHeader(http.StatusTeapot) }))
	defer replacement.Close()
	channel := &model.Channel{Type: constant.ChannelTypeSora, BaseURL: &original.URL, Key: "original-key"}
	require.NoError(t, db.Create(channel).Error)
	info := &relaycommon.RelayInfo{UserId: 1, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: channel.Type, ChannelId: channel.Id, ChannelBaseUrl: original.URL, ApiKey: channel.Key, ChannelTransportFingerprint: channel.TaskTransportFingerprint()}}
	task := model.InitTask(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSora)), info)
	task.TaskID = "task_history"
	task.Status = model.TaskStatusSuccess
	task.PrivateData.UpstreamTaskID = "private-id"
	require.NoError(t, db.Create(task).Error)
	require.NoError(t, db.Model(channel).Updates(map[string]any{"base_url": replacement.URL, "key": "replacement-key"}).Error)
	engine := gin.New()
	engine.GET("/v1/videos/:task_id/content", func(c *gin.Context) { c.Set("id", 1); VideoProxy(c) })
	for _, deleted := range []bool{false, true} {
		if deleted {
			require.NoError(t, db.Delete(channel).Error)
		}
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/videos/task_history/content", nil))
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, "original video", recorder.Body.String())
	}
	require.EqualValues(t, 2, oldRequests.Load())
	require.Zero(t, newRequests.Load())
}

func TestSunoTaskMediaProxyUsesOwnedPersistedAssetsWithoutCredentials(t *testing.T) {
	db := setupTaskResultControllerDB(t)
	var requests atomic.Int32
	original := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		require.Equal(t, "/audio/second", r.URL.Path)
		require.Empty(t, r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("Cookie"))
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("X-Provider", "private")
		w.Header().Set("Set-Cookie", "source-secret=yes")
		_, _ = w.Write([]byte("second song"))
	}))
	defer original.Close()
	task := &model.Task{TaskID: "task_song", UserId: 1, Platform: constant.TaskPlatformSuno, Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{Key: "private-key", BillingContext: &model.TaskBillingContext{SupplierID: 7}}}
	task.SetData([]dto.SunoSong{{AudioURL: original.URL + "/audio/first"}, {AudioURL: original.URL + "/audio/second"}})
	require.NoError(t, db.Create(task).Error)
	engine := gin.New()
	engine.GET("/v1/tasks/:task_id/content", func(c *gin.Context) {
		if c.GetHeader("Test-User") == "other" {
			c.Set("id", 2)
		} else {
			c.Set("id", 1)
		}
		TaskMediaProxy(c)
	})
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/tasks/task_song/content?kind=audio&index=1", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "second song", recorder.Body.String())
	require.Empty(t, recorder.Header().Get("X-Provider"))
	require.Empty(t, recorder.Header().Get("Set-Cookie"))
	require.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	for _, path := range []string{"/v1/tasks/task_song/content?kind=audio&index=2", "/v1/tasks/task_song/content?kind=invalid&index=0"} {
		recorder = httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusNotFound, recorder.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/tasks/task_song/content?kind=audio&index=1", nil)
	req.Header.Set("Test-User", "other")
	recorder = httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.EqualValues(t, 1, requests.Load())
}

func TestCompletedVideoAcceptsRotatedKeyOnlyAtOriginalDestination(t *testing.T) {
	db := setupTaskResultControllerDB(t)
	original := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer rotated-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("video after rotation"))
	}))
	defer original.Close()
	supplier, profile := 1, 2
	channel := &model.Channel{Type: constant.ChannelTypeSora, BaseURL: &original.URL, Key: "revoked-key", SupplierID: &supplier, SupplierProfileID: &profile}
	require.NoError(t, db.Create(channel).Error)
	info := &relaycommon.RelayInfo{UserId: 1, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: channel.Type, ChannelId: channel.Id, ChannelBaseUrl: original.URL, ApiKey: channel.Key, ChannelTransportFingerprint: channel.TaskTransportFingerprint()}}
	task := model.InitTask(constant.TaskPlatform(strconv.Itoa(channel.Type)), info)
	task.TaskID = "task_rotated"
	task.Status = model.TaskStatusSuccess
	task.PrivateData.UpstreamTaskID = "private-id"
	task.PrivateData.BillingContext = &model.TaskBillingContext{SupplierID: supplier, ProfileID: profile}
	require.NoError(t, db.Create(task).Error)
	require.NoError(t, db.Model(channel).Update("key", "rotated-key").Error)
	engine := gin.New()
	engine.GET("/v1/videos/:task_id/content", func(c *gin.Context) { c.Set("id", 1); VideoProxy(c) })
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/videos/task_rotated/content", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "video after rotation", recorder.Body.String())
	changedProfile := 3
	require.NoError(t, db.Model(channel).Update("supplier_profile_id", changedProfile).Error)
	resultChannel, err := model.GetTaskResultChannel(task)
	require.NoError(t, err)
	require.Equal(t, "revoked-key", resultChannel.Key, "a key from a different profile must not be reused for the historical task")
}

func TestCompletedGeminiVideoReplacesRevokedKeyInSavedMediaURL(t *testing.T) {
	db := setupTaskResultControllerDB(t)
	original := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "rotated-key" || r.URL.Query().Get("key") != "rotated-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		require.Equal(t, "media", r.URL.Query().Get("alt"))
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("gemini video after rotation"))
	}))
	defer original.Close()
	channel := &model.Channel{Type: constant.ChannelTypeGemini, BaseURL: &original.URL, Key: "revoked-key"}
	require.NoError(t, db.Create(channel).Error)
	info := &relaycommon.RelayInfo{UserId: 1, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: channel.Type, ChannelId: channel.Id, ChannelBaseUrl: original.URL, ApiKey: channel.Key, ChannelTransportFingerprint: channel.TaskTransportFingerprint()}}
	task := model.InitTask(constant.TaskPlatform(strconv.Itoa(channel.Type)), info)
	task.TaskID = "task_gemini_rotated"
	task.Status = model.TaskStatusSuccess
	task.SetData(map[string]any{"uri": original.URL + "/files/video?key=revoked-key&alt=media"})
	require.NoError(t, db.Create(task).Error)
	require.NoError(t, db.Model(channel).Update("key", "rotated-key").Error)
	engine := gin.New()
	engine.GET("/v1/videos/:task_id/content", func(c *gin.Context) { c.Set("id", 1); VideoProxy(c) })
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/videos/task_gemini_rotated/content", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "gemini video after rotation", recorder.Body.String())
}

func TestCompletedVideoKeepsAcceptedKeyInUnchangedPool(t *testing.T) {
	db := setupTaskResultControllerDB(t)
	original := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer accepted-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte("video from accepted account"))
	}))
	defer original.Close()
	channel := &model.Channel{Type: constant.ChannelTypeSora, BaseURL: &original.URL, Key: "unrelated-key\naccepted-key", ChannelInfo: model.ChannelInfo{IsMultiKey: true}}
	require.NoError(t, db.Create(channel).Error)
	info := &relaycommon.RelayInfo{UserId: 1, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: channel.Type, ChannelId: channel.Id, ChannelBaseUrl: original.URL, ApiKey: "accepted-key", ChannelTransportFingerprint: channel.TaskTransportFingerprint()}}
	task := model.InitTask(constant.TaskPlatform(strconv.Itoa(channel.Type)), info)
	task.TaskID, task.Status = "task_original_key", model.TaskStatusSuccess
	task.PrivateData.UpstreamTaskID = "private-id"
	require.NoError(t, db.Create(task).Error)
	engine := gin.New()
	engine.GET("/v1/videos/:task_id/content", func(c *gin.Context) { c.Set("id", 1); VideoProxy(c) })
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/videos/task_original_key/content", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "video from accepted account", recorder.Body.String())
}

func TestSupplierDataURLMediaUsesSandboxHeaders(t *testing.T) {
	db := setupTaskResultControllerDB(t)
	task := &model.Task{TaskID: "task_html", UserId: 1, Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{ResultURL: "data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==", BillingContext: &model.TaskBillingContext{SupplierID: 1, ProfileID: 2}}}
	require.NoError(t, db.Create(task).Error)
	engine := gin.New()
	engine.GET("/v1/videos/:task_id/content", func(c *gin.Context) { c.Set("id", 1); VideoProxy(c) })
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/videos/task_html/content", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "<script>alert(1)</script>", recorder.Body.String())
	require.Equal(t, "sandbox", recorder.Header().Get("Content-Security-Policy"))
	require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
}
