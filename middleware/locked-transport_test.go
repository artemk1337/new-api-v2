package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestLockedChannelRejectsTransportChangesWithoutReplacingSelection(t *testing.T) {
	for _, change := range []struct {
		name  string
		apply func(*model.Channel)
	}{
		{"url", func(channel *model.Channel) { value := "https://changed.example"; channel.BaseURL = &value }},
		{"key-pool", func(channel *model.Channel) { channel.Key = "first-fixture-key\nchanged-second-key" }},
		{"type", func(channel *model.Channel) { channel.Type = constant.ChannelTypeGemini }},
		{"proxy", func(channel *model.Channel) {
			value := `{"proxy":"http://changed.example:8080"}`
			channel.Setting = &value
		}},
	} {
		t.Run(change.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			baseURL := "https://original.example"
			channel := model.Channel{Id: 42, Type: constant.ChannelTypeOpenAI, BaseURL: &baseURL, Key: "first-fixture-key\nsecond-fixture-key"}
			require.Nil(t, SetupContextForSelectedChannel(ctx, &channel, "model"))
			common.SetContextKey(ctx, constant.ContextKeyChannelKey, "first-fixture-key")
			fingerprint := ctx.GetString("task_channel_transport_fingerprint")
			changed := channel
			change.apply(&changed)

			err := SetupContextForLockedChannel(ctx, &changed, "model")

			require.NotNil(t, err)
			require.Equal(t, fingerprint, ctx.GetString("task_channel_transport_fingerprint"))
			require.Equal(t, baseURL, common.GetContextKeyString(ctx, constant.ContextKeyChannelBaseUrl))
			require.Equal(t, "first-fixture-key", common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
			require.Equal(t, constant.ChannelTypeOpenAI, common.GetContextKeyInt(ctx, constant.ContextKeyChannelType))
		})
	}
}

func TestLockedChannelWithoutTransportSnapshotDoesNotInventOne(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyChannelKey, "previously-selected-key")
	channel := &model.Channel{Id: 42, Type: constant.ChannelTypeOpenAI, Key: "current-key"}

	require.Nil(t, SetupContextForLockedChannel(ctx, channel, "model"))
	require.Empty(t, ctx.GetString("task_channel_transport_fingerprint"))
	require.Equal(t, "previously-selected-key", common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
}
