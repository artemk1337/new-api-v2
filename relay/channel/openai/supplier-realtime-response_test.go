package openai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestSupplierRealtimeHandlerHidesSessionCredentialsAndModel(t *testing.T) {
	upgrade := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrade.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"session.created","event_id":"event_1","metadata":{"source":"private-source"},"session":{"id":"sess_1","model":"private-model","client_secret":{"value":"private-secret"},"provider":"private-source","input_audio_format":"pcm16","output_audio_format":"pcm16"}}`))
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	}))
	defer upstream.Close()
	outcome := make(chan *types.NewAPIError, 1)
	router := gin.New()
	router.GET("/ws", func(c *gin.Context) {
		client, err := upgrade.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer client.Close()
		target, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(upstream.URL, "http"), nil)
		if err != nil {
			return
		}
		defer target.Close()
		info := &relaycommon.RelayInfo{
			ClientWs: client, TargetWs: target, RequestedModelName: "public-alias",
			ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "private-model"},
		}
		info.PriceData.SupplierTariff = &types.SupplierModelTariff{}
		apiErr, _ := OpenaiRealtimeHandler(c, info)
		outcome <- apiErr
	})
	frontend := httptest.NewServer(router)
	defer frontend.Close()
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(frontend.URL, "http")+"/ws", nil)
	require.NoError(t, err)
	defer client.Close()
	require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))

	_, message, err := client.ReadMessage()

	require.NoError(t, err)
	require.Contains(t, string(message), `"model":"public-alias"`)
	require.Contains(t, string(message), `"input_audio_format":"pcm16"`)
	require.NotContains(t, string(message), "private-")
	require.NotContains(t, string(message), "client_secret")
	select {
	case apiErr := <-outcome:
		require.Nil(t, apiErr)
	case <-time.After(5 * time.Second):
		t.Fatal("realtime relay did not finish")
	}
}

func TestSupplierRealtimeProjectionKeepsGeneratedArgumentsAndMasksErrors(t *testing.T) {
	c, _, _, info := newResponsesChatTestContext(t, "", true)
	info.RequestedModelName = "public-alias"
	data, err := projectSupplierRealtimeEvent(c, info, []byte(`{"type":"response.function_call_arguments.delta","event_id":"event_1","delta":"{\"metadata\":\"keep\"}","provider":"private-source"}`))
	require.NoError(t, err)
	require.Contains(t, string(data), `"delta":"{\"metadata\":\"keep\"}"`)
	require.NotContains(t, string(data), "private-")
	data, err = projectSupplierRealtimeEvent(c, info, []byte(`{"type":"error","error":{"message":"private-source credit exhausted","type":"private-provider","metadata":{"cost":0.006}}}`))
	require.NoError(t, err)
	require.Contains(t, string(data), `"code":"request_failed"`)
	require.NotContains(t, string(data), "private-")
	require.NotContains(t, string(data), "metadata")
}
