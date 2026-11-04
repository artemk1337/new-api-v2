package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSelectedTaskTransportIsPersistedWithoutCredentialPool(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	baseURL := "https://source.example"
	channel := &model.Channel{Id: 17, Type: constant.ChannelTypeSora, BaseURL: &baseURL, Key: "first-key\nsecond-key"}
	require.Nil(t, SetupContextForSelectedChannel(ctx, channel, "video-model"))
	common.SetContextKey(ctx, constant.ContextKeyChannelKey, "first-key")
	info := &relaycommon.RelayInfo{}
	info.InitChannelMeta(ctx)
	task := model.InitTask(constant.TaskPlatform("sora"), info)
	require.Equal(t, channel.TaskTransportFingerprint(), task.PrivateData.TransportFingerprint)
	encoded, err := common.Marshal(task)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "first-key")
	require.NotContains(t, string(encoded), task.PrivateData.TransportFingerprint)
	channel.Key = "first-key\nchanged-second-key"
	require.ErrorIs(t, model.ValidateTaskChannelTransport(task, channel), model.ErrTaskChannelTransportChanged)
}
