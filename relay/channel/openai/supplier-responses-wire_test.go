package openai

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

const supplierResponsesDetailedUsage = `{"input_tokens":20,"input_tokens_details":{"cached_tokens":8,"cached_creation_tokens":2,"text_tokens":18,"audio_tokens":0,"image_tokens":2,"cost":0.006,"provider":"private-source"},"output_tokens":12,"output_tokens_details":{"reasoning_tokens":5,"text_tokens":7,"audio_tokens":0,"image_tokens":0,"cost":0.006,"provider":"private-source"},"total_tokens":32,"cost":0.006,"usage_source":"private-source"}`

const supplierResponsesPublicUsage = `{"input_tokens":20,"input_tokens_details":{"cached_tokens":8,"cached_creation_tokens":2,"text_tokens":18,"audio_tokens":0,"image_tokens":2},"output_tokens":12,"output_tokens_details":{"reasoning_tokens":5,"text_tokens":7,"audio_tokens":0,"image_tokens":0},"total_tokens":32}`

func TestSupplierResponsesJSONPreservesCanonicalUsageDetails(t *testing.T) {
	body := `{"id":"resp_continuation","object":"response","created_at":1710000000,"status":"completed","model":"private-model","provider":"private-source","output":[{"type":"reasoning","id":"rs_1","encrypted_content":"opaque-continuation-state"}],"usage":` + supplierResponsesDetailedUsage + `}`
	c, recorder, resp, info := newResponsesChatTestContext(t, body, false)
	info.RelayMode = relayconstant.RelayModeResponses
	info.RequestedModelName = "public-model"
	info.PriceData.SupplierTariff = &types.SupplierModelTariff{}

	usage, apiErr := OaiResponsesHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.Equal(t, 32, usage.TotalTokens)
	var wire map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &wire))
	publicUsage, err := common.Marshal(wire["usage"])
	require.NoError(t, err)
	require.JSONEq(t, supplierResponsesPublicUsage, string(publicUsage))
	require.Equal(t, "resp_continuation", wire["id"])
	require.Equal(t, "public-model", wire["model"])
	require.Contains(t, recorder.Body.String(), "opaque-continuation-state")
	require.NotContains(t, recorder.Body.String(), "private-")
	require.NotContains(t, recorder.Body.String(), `"cost"`)
}

func TestSupplierResponsesSSEPreservesRefusalsAnnotationsAndUsage(t *testing.T) {
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	body := strings.Join([]string{
		`data: {"type":"response.refusal.done","sequence_number":0,"item_id":"msg_1","output_index":0,"content_index":0,"refusal":"Cannot comply","provider":"private-source"}`,
		`data: {"type":"response.refusal.done","item_id":"msg_2","refusal":"","provider":"private-source"}`,
		`data: {"type":"response.output_text.annotation.added","item_id":"msg_1","output_index":0,"content_index":0,"annotation_index":0,"annotation":{"type":"url_citation","url":"https://example.com","title":"Canonical cite","start_index":0,"end_index":4},"provider":"private-source"}`,
		`data: {"type":"response.completed","response":{"id":"resp_continuation","object":"response","status":"completed","model":"private-model","output":[{"type":"reasoning","id":"rs_1","encrypted_content":"opaque-continuation-state"}],"usage":` + supplierResponsesDetailedUsage + `}}`,
		`data: [DONE]`, ``,
	}, "\n")
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	info.RelayMode = relayconstant.RelayModeResponses
	info.RequestedModelName = "public-model"
	info.PriceData.SupplierTariff = &types.SupplierModelTariff{}

	usage, apiErr := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.Equal(t, 32, usage.TotalTokens)
	var events []map[string]any
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") || line == "data: [DONE]" {
			continue
		}
		var event map[string]any
		require.NoError(t, common.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
		events = append(events, event)
	}
	require.Len(t, events, 4)
	require.Equal(t, "Cannot comply", events[0]["refusal"])
	require.Equal(t, float64(0), events[0]["sequence_number"])
	require.Contains(t, events[1], "refusal")
	require.Equal(t, "", events[1]["refusal"])
	require.Equal(t, float64(0), events[2]["annotation_index"])
	require.Equal(t, "Canonical cite", events[2]["annotation"].(map[string]any)["title"])
	response := events[3]["response"].(map[string]any)
	publicUsage, err := common.Marshal(response["usage"])
	require.NoError(t, err)
	require.JSONEq(t, supplierResponsesPublicUsage, string(publicUsage))
	require.Equal(t, "resp_continuation", response["id"])
	require.Equal(t, "public-model", response["model"])
	require.Contains(t, recorder.Body.String(), "opaque-continuation-state")
	require.NotContains(t, recorder.Body.String(), "private-")
	require.NotContains(t, recorder.Body.String(), `"cost"`)
}
