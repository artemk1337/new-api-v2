package gemini

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskAdaptorEstimateBilling(t *testing.T) {
	tests := []struct {
		name          string
		originModel   string
		upstreamModel string
		req           common.TaskSubmitReq
		want          map[string]float64
	}{
		{
			name:          "OpenLux request includes duration ratio",
			originModel:   "veo_3_1",
			upstreamModel: "veo_3_1",
			req:           common.TaskSubmitReq{Duration: 4},
			want:          map[string]float64{"seconds": 4, "resolution": 1},
		},
		{
			name:          "OpenLux fast request includes duration ratio",
			originModel:   "veo_3_1-fast",
			upstreamModel: "veo_3_1-fast",
			req:           common.TaskSubmitReq{Duration: 6},
			want:          map[string]float64{"seconds": 6, "resolution": 1},
		},
		{
			name:          "Google per second price",
			originModel:   "veo-3.1-generate-preview",
			upstreamModel: "veo-3.1-generate-preview",
			req:           common.TaskSubmitReq{Duration: 4},
			want:          map[string]float64{"seconds": 4, "resolution": 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Set("task_request", tt.req)

			ratios := (&TaskAdaptor{}).EstimateBilling(ctx, &common.RelayInfo{
				OriginModelName: tt.originModel,
				ChannelMeta: &common.ChannelMeta{
					UpstreamModelName: tt.upstreamModel,
				},
			})

			require.NotNil(t, ratios)
			assert.Equal(t, tt.want, ratios)
		})
	}
}
