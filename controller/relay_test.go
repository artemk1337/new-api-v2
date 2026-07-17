package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRespondTaskErrorUsesEnglishGroupSaturationMessage(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	respondTaskError(ctx, &dto.TaskError{
		Code:       "429",
		Message:    "upstream overloaded",
		StatusCode: http.StatusTooManyRequests,
	})

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	assert.JSONEq(t, `{
		"code": "429",
		"message": "The upstream load for the current group is saturated. Please try again later or switch to another group.",
		"data": null
	}`, recorder.Body.String())
}

func TestRelayDisplayErrorMessageNormalizesKnownUpstreamErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		message    string
		expected   string
	}{
		{
			name:       "group upstream saturated",
			statusCode: http.StatusTooManyRequests,
			message:    "当前分组上游负载已饱和，请稍后再试 (request id: upstream)",
			expected:   relayGroupUpstreamSaturatedMessage,
		},
		{
			name:       "model upstream saturated",
			statusCode: http.StatusTooManyRequests,
			message:    "当前模型gpt-image-1.5上游已饱和, 请稍后再试! (request id: upstream)",
			expected:   relayGroupUpstreamSaturatedMessage,
		},
		{
			name:       "credentials concurrency",
			statusCode: http.StatusTooManyRequests,
			message:    "所有可用凭据均已达到并发上限，请稍后重试。 [up_rate_limit]",
			expected:   relayCredentialsConcurrencyMessage,
		},
		{
			name:       "empty upstream response",
			statusCode: http.StatusBadRequest,
			message:    "未接收到上游响应内容（traceid: abc）",
			expected:   relayNoUpstreamContentMessage,
		},
		{
			name:       "no upstream token",
			statusCode: http.StatusBadRequest,
			message:    "没有可用token（traceid: abc）",
			expected:   relayNoUpstreamTokenMessage,
		},
		{
			name:       "empty model content",
			statusCode: http.StatusInternalServerError,
			message:    "模型返回内容为空",
			expected:   relayEmptyModelContentMessage,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := types.NewErrorWithStatusCode(errors.New(test.message), types.ErrorCodeBadResponse, test.statusCode)

			message, normalized := relayDisplayErrorMessage(err)

			require.True(t, normalized)
			assert.Equal(t, test.expected, message)
		})
	}
}

func TestRelayErrorLogContentNormalizesAndKeepsRawDiagnostic(t *testing.T) {
	err := types.NewErrorWithStatusCode(
		errors.New("当前分组上游负载已饱和，请稍后再试 (request id: upstream)"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusTooManyRequests,
	)

	content, rawDiagnostic, normalized := relayErrorLogContent(err)

	require.True(t, normalized)
	assert.Equal(t, "status_code=429, "+relayGroupUpstreamSaturatedMessage, content)
	assert.Equal(t, "status_code=429, 当前分组上游负载已饱和，请稍后再试 (request id: upstream)", rawDiagnostic)
}

func TestPrepareRelayResponseErrorUpdatesOpenAIRelayMessage(t *testing.T) {
	err := types.WithOpenAIError(types.OpenAIError{
		Message: "当前分组上游负载已饱和，请稍后再试",
		Type:    "upstream_error",
		Code:    "429",
	}, http.StatusTooManyRequests)

	prepareRelayResponseError(err, "req-1")

	openAIError := err.ToOpenAIError()
	assert.Equal(t, relayGroupUpstreamSaturatedMessage+" (request id: req-1)", openAIError.Message)
	assert.False(t, strings.Contains(openAIError.Message, "当前分组"))
}

func TestErrorLogPricingGroupUsesSelectedPricingGroup(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("group", "user-group")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "2")

	assert.Equal(t, "2", errorLogPricingGroup(ctx))
}

func TestErrorLogPricingGroupUsesAutoGroupWhenSelected(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "1")
	common.SetContextKey(ctx, constant.ContextKeyAutoGroup, "3")

	assert.Equal(t, "3", errorLogPricingGroup(ctx))
}
