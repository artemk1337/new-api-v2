package dto

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestOutputTokenDetailsTracksReasoningTokenPresence(t *testing.T) {
	var withReasoning OutputTokenDetails
	require.NoError(t, common.Unmarshal([]byte(`{"reasoning_tokens":0}`), &withReasoning))
	require.True(t, withReasoning.ReasoningTokensPresent)

	var withoutReasoning OutputTokenDetails
	require.NoError(t, common.Unmarshal([]byte(`{"text_tokens":10}`), &withoutReasoning))
	require.False(t, withoutReasoning.ReasoningTokensPresent)

	var nullReasoning OutputTokenDetails
	require.NoError(t, common.Unmarshal([]byte(`{"reasoning_tokens":null}`), &nullReasoning))
	require.False(t, nullReasoning.ReasoningTokensPresent)
}
