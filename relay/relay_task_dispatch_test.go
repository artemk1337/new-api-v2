package relay

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDispatchTaskRequestMarksTransportErrorAmbiguous(t *testing.T) {
	info := &relaycommon.RelayInfo{}
	transportErr := errors.New("connection reset")

	_, err := dispatchTaskRequest(info, func() (*http.Response, error) {
		assert.True(t, info.AttemptDispatched)
		return nil, transportErr
	})

	require.ErrorIs(t, err, transportErr)
	taskErr := &dto.TaskError{}
	assert.Equal(t, types.AttemptFinancialOutcomeAmbiguous, service.ClassifyTaskAttempt(info, taskErr))
	assert.False(t, service.ShouldRetryAutoAttempt(
		types.AttemptFinancialOutcomeAmbiguous,
		info.AttemptDispatched,
		true,
		true,
	))
}

func TestApplyTaskPriceUnit(t *testing.T) {
	original := billing_setting.GetTaskPriceUnitCopy()
	originalJSON, err := common.Marshal(original)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, billing_setting.UpdateFromMap(map[string]string{
			billing_setting.TaskPriceUnitField: string(originalJSON),
		}))
	})

	require.NoError(t, billing_setting.UpdateFromMap(map[string]string{
		billing_setting.TaskPriceUnitField: `{"per-call-model":"per_call"}`,
	}))

	perCallRatios := map[string]float64{"seconds": 4, "resolution": 1.5}
	applyTaskPriceUnit("per-call-model", perCallRatios)
	assert.Equal(t, map[string]float64{"resolution": 1.5}, perCallRatios)

	perSecondRatios := map[string]float64{"seconds": 4, "resolution": 1.5}
	applyTaskPriceUnit("per-second-model", perSecondRatios)
	assert.Equal(t, map[string]float64{"seconds": 4, "resolution": 1.5}, perSecondRatios)
}
