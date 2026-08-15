package ali

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdaptorDoesNotSupportAudioSpeech(t *testing.T) {
	require.False(t, (&Adaptor{}).SupportsAudioSpeech())
}
