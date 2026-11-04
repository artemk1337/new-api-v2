package openai

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestSupplierImageJSONKeepsGeneratedImageAndPrivateUsageSeparate(t *testing.T) {
	body := `{"created":1710000000,"metadata":{"source":"private-source"},"data":[{"b64_json":"generated-image-data","revised_prompt":"Keep metadata in generated prompt","provider":"private-source"}],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5,"cost":0.006}}`
	c, recorder, resp, info := newResponsesChatTestContext(t, body, false)
	info.PriceData.SupplierTariff = &types.SupplierModelTariff{}
	resp.Header.Set("Server", "private-source")

	usage, apiErr := OpenaiImageHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.Equal(t, 5, usage.TotalTokens)
	require.Equal(t, 0.006, usage.Cost)
	require.Contains(t, recorder.Body.String(), "generated-image-data")
	require.Contains(t, recorder.Body.String(), "Keep metadata in generated prompt")
	require.NotContains(t, recorder.Body.String(), "private-")
	require.NotContains(t, recorder.Body.String(), `"cost"`)
	require.Empty(t, recorder.Header().Get("Server"))
}

func TestSupplierImageSSEAndJSONFallbackStripServiceFields(t *testing.T) {
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	for _, isSSE := range []bool{true, false} {
		body := `{"created":1710000000,"metadata":{"source":"private-source"},"data":[{"b64_json":"generated-image-data"}],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5,"cost":0.006}}`
		if isSSE {
			body = "data: " + `{"type":"image_generation.completed","created_at":1710000000,"b64_json":"generated-image-data","metadata":{"source":"private-source"},"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5,"cost":0.006}}` + "\n\ndata: [DONE]\n"
		}
		c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
		info.PriceData.SupplierTariff = &types.SupplierModelTariff{}
		if !isSSE {
			resp.Header.Set("Content-Type", "application/json")
		}
		usage, apiErr := OpenaiImageStreamHandler(c, info, resp)
		require.Nil(t, apiErr)
		require.Equal(t, 5, usage.TotalTokens)
		require.Equal(t, 0.006, usage.Cost)
		require.Contains(t, recorder.Body.String(), "generated-image-data")
		require.NotContains(t, recorder.Body.String(), "private-")
		require.NotContains(t, recorder.Body.String(), `"cost"`)
	}
}

func TestSupplierImageSSEErrorDoesNotExposeProvider(t *testing.T) {
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	body := "data: " + `{"type":"error","message":"private-source credit exhausted","error":{"metadata":{"cost":0.006}}}` + "\n\ndata: [DONE]\n"
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	info.PriceData.SupplierTariff = &types.SupplierModelTariff{}

	_, apiErr := OpenaiImageStreamHandler(c, info, resp)

	require.NotNil(t, apiErr)
	require.Contains(t, recorder.Body.String(), "Request could not be completed")
	require.NotContains(t, recorder.Body.String(), "private-")
	require.NotContains(t, recorder.Body.String(), "0.006")
}

func TestSupplierAudioJSONPreservesTranscriptAndTiming(t *testing.T) {
	body := `{"text":"Keep provider metadata in transcribed content","language":"en","duration":1.5,"words":[{"word":"Keep","start":0,"end":0.2}],"segments":[{"id":0,"text":"Keep provider metadata","start":0,"end":1.5}],"metadata":{"source":"private-source"},"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5,"cost":0.006}}`
	c, recorder, resp, info := newResponsesChatTestContext(t, body, false)
	info.PriceData.SupplierTariff = &types.SupplierModelTariff{}
	resp.Header.Set("X-Provider", "private-source")

	apiErr, usage := OpenaiSTTHandler(c, resp, info, "verbose_json")

	require.Nil(t, apiErr)
	require.Equal(t, 5, usage.TotalTokens)
	require.Equal(t, 0.006, usage.Cost)
	require.Contains(t, recorder.Body.String(), "Keep provider metadata in transcribed content")
	require.Contains(t, recorder.Body.String(), `"duration":1.5`)
	require.Contains(t, recorder.Body.String(), `"word":"Keep"`)
	require.NotContains(t, recorder.Body.String(), "private-")
	require.NotContains(t, recorder.Body.String(), `"cost"`)
	require.Empty(t, recorder.Header().Get("X-Provider"))
}

func TestSupplierAudioSSEKeepsAudioAndHidesSourceCost(t *testing.T) {
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	body := "data: " + `{"type":"speech.audio.delta","audio":"generated-audio-data","metadata":{"source":"private-source"}}` + "\n\ndata: " + `{"type":"speech.audio.done","usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5,"cost":0.006}}` + "\n\ndata: [DONE]\n"
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	info.PriceData.SupplierTariff = &types.SupplierModelTariff{}

	usage := OpenaiTTSHandler(c, resp, info)

	require.Equal(t, 5, usage.TotalTokens)
	require.Contains(t, recorder.Body.String(), "generated-audio-data")
	require.NotContains(t, recorder.Body.String(), "private-")
	require.NotContains(t, recorder.Body.String(), `"cost"`)
}

func TestSupplierBinaryAudioAndPlainTranscriptAreUnchanged(t *testing.T) {
	binary := []byte{0x00, 0x03, 0xff, 0x0a}
	c, recorder, resp, info := newResponsesChatTestContext(t, "", false)
	resp.Body = io.NopCloser(bytes.NewReader(binary))
	resp.Header.Set("Content-Type", "audio/pcm")
	resp.Header.Set("Server", "private-source")
	info.Request = &dto.AudioRequest{ResponseFormat: "pcm"}
	info.PriceData.SupplierTariff = &types.SupplierModelTariff{}
	OpenaiTTSHandler(c, resp, info)
	require.Equal(t, binary, recorder.Body.Bytes())
	require.Empty(t, recorder.Header().Get("Server"))

	transcript := "metadata and provider are words in user audio"
	c, recorder, resp, info = newResponsesChatTestContext(t, transcript, false)
	resp.Header.Set("Content-Type", "text/plain")
	info.PriceData.SupplierTariff = &types.SupplierModelTariff{}
	apiErr, _ := OpenaiSTTHandler(c, resp, info, "text")
	require.Nil(t, apiErr)
	require.Equal(t, transcript, strings.TrimSpace(recorder.Body.String()))
}
