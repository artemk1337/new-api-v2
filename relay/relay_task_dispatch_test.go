package relay

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
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
