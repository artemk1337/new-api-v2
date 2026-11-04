package relay

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTaskActionsAndRealtimeFetchBlockChangedTransport(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "task-transport.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Task{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB; sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests.Add(1); w.WriteHeader(http.StatusOK) }))
	defer upstream.Close()
	channel := &model.Channel{Type: constant.ChannelTypeGemini, BaseURL: &upstream.URL, Key: "old-private-key"}
	require.NoError(t, db.Create(channel).Error)
	fingerprint := channel.TaskTransportFingerprint()
	require.NoError(t, db.Model(channel).Update("key", "replacement-private-key").Error)
	task := &model.Task{TaskID: "task_action_race", UserId: 1, ChannelId: channel.Id, Status: model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "private-original-id", Key: "old-private-key", TransportFingerprint: fingerprint}}
	require.NoError(t, db.Create(task).Error)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/task_action_race/remix", nil)
	ctx.Params = gin.Params{{Key: "video_id", Value: task.TaskID}}
	info := &relaycommon.RelayInfo{UserId: 1, UsingGroup: "1", TaskRelayInfo: &relaycommon.TaskRelayInfo{OriginTaskID: task.TaskID}}
	taskError := ResolveOriginTask(ctx, info)
	require.NotNil(t, taskError)
	require.False(t, info.AttemptDispatched)
	require.Empty(t, tryRealtimeFetch(task, false))
	require.Zero(t, requests.Load())
}
