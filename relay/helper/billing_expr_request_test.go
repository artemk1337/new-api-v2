package helper

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResolveIncomingBillingExprRequestInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("Content-Type", "application/json")

	body := []byte(`{"service_tier":"fast"}`)
	ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
	ctx.Set(common.KeyRequestBody, body)

	info := &relaycommon.RelayInfo{
		RequestHeaders: map[string]string{"Content-Type": "application/json"},
	}

	input, err := ResolveIncomingBillingExprRequestInput(ctx, info)
	require.NoError(t, err)
	require.Equal(t, body, input.Body)
	require.Equal(t, "application/json", input.Headers["Content-Type"])
}

func TestImageEditMultipartBillingQuality(t *testing.T) {
	gin.SetMode(gin.TestMode)
	expr := `has(param("quality"), "4k") ? tier("4k", 2800000) : has(param("quality"), "2k") ? tier("2k", 2300000) : tier("1k", 1700000)`
	for _, tc := range []struct {
		quality string
		want    float64
	}{{"", 1700000}, {"2k", 2300000}, {"4k", 2800000}} {
		t.Run(tc.quality, func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			require.NoError(t, writer.WriteField("quality", tc.quality))
			file, err := writer.CreateFormFile("image", "image.png")
			require.NoError(t, err)
			_, err = file.Write([]byte("image data"))
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
			ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
			input, err := ResolveIncomingBillingExprRequestInput(ctx, &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits})
			require.NoError(t, err)
			cost, _, err := billingexpr.RunExprWithRequest(expr, billingexpr.TokenParams{}, input)
			require.NoError(t, err)
			require.Equal(t, tc.want, cost)
			require.False(t, gjson.GetBytes(input.Body, "image").Exists())
			form, err := common.ParseMultipartFormReusable(ctx)
			require.NoError(t, err)
			defer form.RemoveAll()
			require.Equal(t, tc.quality, form.Value["quality"][0])
			require.Len(t, form.File["image"], 1)
		})
	}
}

func TestBuildBillingExprRequestInputFromRequest(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model:  "gemini-3.1-pro-preview",
		Stream: lo.ToPtr(true),
		Messages: []dto.Message{
			{
				Role:    "user",
				Content: "hi",
			},
		},
		MaxTokens: lo.ToPtr(uint(3000)),
	}

	input, err := BuildBillingExprRequestInputFromRequest(request, map[string]string{
		"Content-Type": "application/json",
		"X-Test":       "1",
	})
	require.NoError(t, err)
	require.Equal(t, "application/json", input.Headers["Content-Type"])
	require.Equal(t, "1", input.Headers["X-Test"])
	require.True(t, gjson.GetBytes(input.Body, "stream").Bool())
	require.Equal(t, "user", gjson.GetBytes(input.Body, "messages.0.role").String())
	require.Equal(t, float64(3000), gjson.GetBytes(input.Body, "max_tokens").Float())
}
