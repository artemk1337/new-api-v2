package relay

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSupplierSunoSubmitKeepsNativeTaskIDContract(t *testing.T) {
	service.InitHttpClient()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/suno/submit/MUSIC", r.URL.Path)
		require.Equal(t, "Bearer private-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"success","message":"private-source","data":"private-id"}`))
	}))
	defer upstream.Close()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/suno/submit/MUSIC", strings.NewReader(`{"prompt":"a song"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "action", Value: constant.SunoActionMusic}}
	ctx.Set("platform", string(constant.TaskPlatformSuno))
	ctx.Set("supplier_pricing", true)
	ctx.Set("task_channel_transport_fingerprint", "frozen")
	common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeSunoAPI)
	common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, upstream.URL)
	common.SetContextKey(ctx, constant.ContextKeyChannelKey, "private-key")
	zero := 0.0
	info := &relaycommon.RelayInfo{OriginModelName: "suno_music", UsingGroup: "1", TokenGroup: "1", TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}, PriceData: types.PriceData{UsePrice: true, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 2}, SupplierTariff: &types.SupplierModelTariff{BillingMode: types.SupplierBillingModeFixed, PriceUnit: "request", PriceUSD: &zero}}}
	result, taskError := RelayTaskSubmit(ctx, info)
	require.Nil(t, taskError)
	require.Equal(t, "private-id", result.UpstreamTaskID)
	var response dto.TaskResponse[string]
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, dto.TaskSuccessCode, response.Code)
	require.Equal(t, "task_public", response.Data)
	require.NotContains(t, recorder.Body.String(), "private-")
}

func TestSupplierSunoFetchKeepsSongsAndLyricsPrivateMedia(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "suno.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB; connection, _ := db.DB(); _ = connection.Close() })
	private := model.TaskPrivateData{BillingContext: &model.TaskBillingContext{SupplierID: 1, ProfileID: 2}}
	music := &model.Task{TaskID: "task_music", UserId: 1, Platform: constant.TaskPlatformSuno, Action: constant.SunoActionMusic, Status: model.TaskStatusSuccess, PrivateData: private}
	music.SetData([]dto.SunoSong{
		{ID: "clip1", AudioURL: "https://private-supplier/audio1", ImageURL: "https://private-supplier/image1", Text: "first song", ModelName: "private-model", Metadata: dto.SunoMetadata{GPTDescriptionPrompt: "a melody", AudioPromptID: "prompt1", Duration: 123.4}},
		{ID: "clip2", AudioURL: "https://private-supplier/audio2", Text: "second song", Metadata: dto.SunoMetadata{GPTDescriptionPrompt: map[string]any{"supplier": "private-supplier"}, AudioPromptID: map[string]any{"key": "private-key"}, Duration: []any{"private-source"}, ErrorType: "private-error", ErrorMessage: "private-error"}},
	})
	lyrics := &model.Task{TaskID: "task_lyrics", UserId: 1, Platform: constant.TaskPlatformSuno, Action: constant.SunoActionLyrics, Status: model.TaskStatusSuccess, PrivateData: private}
	lyrics.SetData(dto.SunoLyrics{ID: "lyrics1", Title: "A title", Text: "generated lyrics", Status: "complete"})
	require.NoError(t, db.Create(music).Error)
	require.NoError(t, db.Create(lyrics).Error)
	engine := gin.New()
	engine.GET("/suno/fetch/:id", func(c *gin.Context) {
		c.Set("id", 1)
		require.Nil(t, RelayTaskFetch(c, relayconstant.RelayModeSunoFetchByID))
	})
	engine.POST("/suno/fetch", func(c *gin.Context) {
		c.Set("id", 1)
		require.Nil(t, RelayTaskFetch(c, relayconstant.RelayModeSunoFetch))
	})
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/suno/fetch/task_music", nil))
	var fetched dto.TaskResponse[dto.TaskDto]
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &fetched))
	var songs []dto.SunoSong
	require.NoError(t, common.Unmarshal(fetched.Data.Data, &songs))
	require.Len(t, songs, 2)
	require.Equal(t, "clip1", songs[0].ID)
	require.Equal(t, "second song", songs[1].Text)
	require.Equal(t, "a melody", songs[0].Metadata.GPTDescriptionPrompt)
	require.Equal(t, "prompt1", songs[0].Metadata.AudioPromptID)
	require.Equal(t, 123.4, songs[0].Metadata.Duration)
	require.Nil(t, songs[1].Metadata.GPTDescriptionPrompt)
	require.Nil(t, songs[1].Metadata.AudioPromptID)
	require.Nil(t, songs[1].Metadata.Duration)
	require.Contains(t, songs[0].AudioURL, "/v1/tasks/task_music/content?kind=audio&index=0")
	require.Contains(t, songs[1].AudioURL, "index=1")
	require.Empty(t, fetched.Data.ResultURL)
	require.NotContains(t, recorder.Body.String(), "private-supplier")
	require.NotContains(t, recorder.Body.String(), "private-model")
	require.NotContains(t, recorder.Body.String(), "private-key")
	require.NotContains(t, recorder.Body.String(), "private-source")
	require.NotContains(t, recorder.Body.String(), "private-error")
	recorder = httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/suno/fetch", strings.NewReader(`{"ids":["task_music","task_lyrics"]}`)))
	var multiple dto.TaskResponse[[]dto.TaskDto]
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &multiple))
	require.Len(t, multiple.Data, 2)
	var storedLyrics dto.SunoLyrics
	for _, task := range multiple.Data {
		if task.TaskID == "task_lyrics" {
			require.NoError(t, common.Unmarshal(task.Data, &storedLyrics))
		}
	}
	require.Equal(t, "generated lyrics", storedLyrics.Text)
	require.Equal(t, "A title", storedLyrics.Title)
}

func TestFinishedVideoStatusDoesNotRequireCurrentChannel(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "status.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB; connection, _ := db.DB(); _ = connection.Close() })
	for _, bound := range []bool{false, true} {
		id := "task_unbound"
		private := model.TaskPrivateData{TransportFingerprint: "old-channel-digest"}
		if bound {
			id = "task_bound"
			private.BillingContext = &model.TaskBillingContext{SupplierID: 1, ProfileID: 2}
		}
		task := &model.Task{TaskID: id, UserId: 1, ChannelId: 987, Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSora)), Status: model.TaskStatusSuccess, PrivateData: private}
		task.SetData(map[string]any{"id": "private-id", "model": "sora", "status": "completed"})
		require.NoError(t, db.Create(task).Error)
		engine := gin.New()
		engine.GET("/v1/videos/:task_id", func(c *gin.Context) {
			c.Set("id", 1)
			require.Nil(t, RelayTaskFetch(c, relayconstant.RelayModeVideoFetchByID))
		})
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/videos/"+id, nil))
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Contains(t, recorder.Body.String(), id)
	}
}
