package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayErrorHandlerFinancialOutcome(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		outcome types.AttemptFinancialOutcome
	}{
		{
			name:    "structured 429 without explicit no charge is unknown",
			status:  http.StatusTooManyRequests,
			body:    `{"error":{"message":"rate limited","data":{"retry_after":1}}}`,
			outcome: types.AttemptFinancialOutcomeUnknown,
		},
		{
			name:    "known group saturation is non billable",
			status:  http.StatusTooManyRequests,
			body:    `{"error":{"message":"The upstream load for the current group is saturated. Please try again later or switch to another group."}}`,
			outcome: types.AttemptFinancialOutcomeNonBillable,
		},
		{
			name:    "known model saturation is non billable",
			status:  http.StatusTooManyRequests,
			body:    `{"message":"当前模型wan2.6-i2v上游已饱和, 请稍后再试!"}`,
			outcome: types.AttemptFinancialOutcomeNonBillable,
		},
		{
			name:    "known credentials concurrency saturation is non billable",
			status:  http.StatusTooManyRequests,
			body:    `{"error":{"message":"所有可用凭据均已达到并发上限，请稍后重试。 [up_rate_limit]"}}`,
			outcome: types.AttemptFinancialOutcomeNonBillable,
		},
		{
			name:    "structured 503 without explicit no charge is unknown",
			status:  http.StatusServiceUnavailable,
			body:    `{"message":"temporarily unavailable"}`,
			outcome: types.AttemptFinancialOutcomeUnknown,
		},
		{
			name:    "usage is billable",
			status:  http.StatusTooManyRequests,
			body:    `{"error":{"message":"failed"},"usage":{"prompt_tokens":12}}`,
			outcome: types.AttemptFinancialOutcomeBillable,
		},
		{
			name:    "partial output is billable",
			status:  http.StatusServiceUnavailable,
			body:    `{"error":{"message":"failed"},"output":[{"text":"partial"}]}`,
			outcome: types.AttemptFinancialOutcomeBillable,
		},
		{
			name:    "accepted task is billable",
			status:  http.StatusServiceUnavailable,
			body:    `{"error":{"message":"poll failed"},"task_id":"task-1"}`,
			outcome: types.AttemptFinancialOutcomeBillable,
		},
		{
			name:    "explicit zero cost remains free",
			status:  http.StatusInternalServerError,
			body:    `{"error":{"message":"rejected"},"cost":0}`,
			outcome: types.AttemptFinancialOutcomeNonBillable,
		},
		{
			name:    "explicit charged false remains free",
			status:  http.StatusInternalServerError,
			body:    `{"error":{"message":"rejected"},"charged":false}`,
			outcome: types.AttemptFinancialOutcomeNonBillable,
		},
		{
			name:    "explicit charged true is billable",
			status:  http.StatusTooManyRequests,
			body:    `{"error":{"message":"rate limited"},"charged":true}`,
			outcome: types.AttemptFinancialOutcomeBillable,
		},
		{
			name:    "explicit billed true is billable",
			status:  http.StatusServiceUnavailable,
			body:    `{"error":{"message":"temporarily unavailable"},"billing":{"billed":true}}`,
			outcome: types.AttemptFinancialOutcomeBillable,
		},
		{
			name:    "billable true wins over charged false",
			status:  http.StatusTooManyRequests,
			body:    `{"error":{"message":"rate limited"},"charged":false,"billable":true}`,
			outcome: types.AttemptFinancialOutcomeBillable,
		},
		{
			name:    "nested metadata zero cost is not authoritative",
			status:  http.StatusInternalServerError,
			body:    `{"error":{"message":"failed"},"metadata":{"cost":0}}`,
			outcome: types.AttemptFinancialOutcomeUnknown,
		},
		{
			name:    "direct error output is billable",
			status:  http.StatusServiceUnavailable,
			body:    `{"error":{"message":"failed","output":"partial"}}`,
			outcome: types.AttemptFinancialOutcomeBillable,
		},
		{
			name:    "direct error task id is billable",
			status:  http.StatusTooManyRequests,
			body:    `{"error":{"message":"failed","task_id":"task-1"}}`,
			outcome: types.AttemptFinancialOutcomeBillable,
		},
		{
			name:    "correlation id is not billing evidence",
			status:  http.StatusTooManyRequests,
			body:    `{"error":{"message":"rate limited"},"id":"req-123"}`,
			outcome: types.AttemptFinancialOutcomeUnknown,
		},
		{
			name:    "error correlation id is not billing evidence",
			status:  http.StatusTooManyRequests,
			body:    `{"error":{"message":"rate limited","id":"req-123"}}`,
			outcome: types.AttemptFinancialOutcomeUnknown,
		},
		{
			name:    "false result is not partial output",
			status:  http.StatusTooManyRequests,
			body:    `{"error":{"message":"rate limited"},"result":false}`,
			outcome: types.AttemptFinancialOutcomeUnknown,
		},
		{
			name:    "false output is not partial output",
			status:  http.StatusServiceUnavailable,
			body:    `{"error":{"message":"temporarily unavailable"},"output":false}`,
			outcome: types.AttemptFinancialOutcomeUnknown,
		},
		{
			name:    "zero result is not partial output",
			status:  http.StatusTooManyRequests,
			body:    `{"error":{"message":"rate limited"},"result":0}`,
			outcome: types.AttemptFinancialOutcomeUnknown,
		},
		{
			name:    "unstructured body remains unknown",
			status:  http.StatusServiceUnavailable,
			body:    `<html>bad gateway</html>`,
			outcome: types.AttemptFinancialOutcomeUnknown,
		},
		{
			name:    "empty object remains unknown",
			status:  http.StatusServiceUnavailable,
			body:    `{}`,
			outcome: types.AttemptFinancialOutcomeUnknown,
		},
		{
			name:    "generic structured 500 remains unknown",
			status:  http.StatusInternalServerError,
			body:    `{"error":{"message":"upstream failed"}}`,
			outcome: types.AttemptFinancialOutcomeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				StatusCode: tt.status,
				Body:       io.NopCloser(strings.NewReader(tt.body)),
			}

			err := RelayErrorHandler(t.Context(), resp, false)

			assert.Equal(t, tt.outcome, err.GetFinancialOutcome())
		})
	}
}

func TestRelayErrorHandlerNormalizesSaturatedBadGateway(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		status int
	}{
		{
			name:   "group saturation",
			body:   `{"error":{"message":"The upstream load for the current group is saturated."}}`,
			status: http.StatusTooManyRequests,
		},
		{
			name:   "upstream overloaded",
			body:   `{"message":"upstream overloaded, please try again later"}`,
			status: http.StatusTooManyRequests,
		},
		{
			name:   "ordinary bad gateway",
			body:   `{"error":{"message":"bad gateway"}}`,
			status: http.StatusBadGateway,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				StatusCode: http.StatusBadGateway,
				Body:       io.NopCloser(strings.NewReader(tt.body)),
			}

			err := RelayErrorHandler(t.Context(), resp, false)
			require.NotNil(t, err)
			assert.Equal(t, tt.status, err.StatusCode)
			if tt.status == http.StatusTooManyRequests {
				assert.Equal(t, types.AttemptFinancialOutcomeNonBillable, err.GetFinancialOutcome())
			}
		})
	}
}

func TestKnownSaturationEnablesAutoFallbackAfterDispatch(t *testing.T) {
	outcome := ClassifyUpstreamErrorResponse(http.StatusTooManyRequests, []byte(`{"error":{"message":"The upstream load for the current group is saturated."}}`))
	assert.True(t, ShouldRetryAutoAttempt(outcome, true, false, true))
}

func TestRelayErrorHandlerTreatsResourceIdentifiersAsBillingEvidence(t *testing.T) {
	bodies := []string{
		`{"error":{"message":"failed"},"task_id":"task-1"}`,
		`{"error":{"message":"failed"},"taskid":"task-1"}`,
		`{"error":{"message":"failed"},"job_id":"job-1"}`,
		`{"error":{"message":"failed"},"operation_id":"op-1"}`,
		`{"error":{"message":"failed"},"operation":{"name":"operations/1"}}`,
		`{"error":{"message":"failed"},"prediction_id":"prediction-1"}`,
	}
	for _, body := range bodies {
		t.Run(body, func(t *testing.T) {
			assert.Equal(t,
				types.AttemptFinancialOutcomeBillable,
				ClassifyUpstreamErrorResponse(http.StatusServiceUnavailable, []byte(body)),
			)
		})
	}
}

func TestRelayErrorHandlerCapturesAuthoritativeCostAboveReserve(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		StartTime:  time.Now(),
		UsingGroup: "budget",
		PriceData: types.PriceData{
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 2},
		},
		AutoRoute: relaycommon.AutoRouteState{
			ReservedQuota: 100,
			Candidates: []relaycommon.AutoRouteCandidate{
				{Group: "budget", EstimatedQuota: 100},
			},
		},
	}
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"failed"},"cost":0.002}`)),
	}

	apiErr := RelayErrorHandler(t.Context(), resp, false)
	require.True(t, CaptureErrorBillingQuota(ctx, info, apiErr))

	expected := int(0.002 * common.QuotaPerUnit * 2)
	assert.Equal(t, expected, info.AttemptActualQuota)
	assert.Greater(t, info.AttemptActualQuota, info.AutoRoute.ReservedQuota)
	assert.Equal(t, expected, chargedAttemptQuota(info))
}
