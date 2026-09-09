package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFlexibleStringUnmarshal(t *testing.T) {
	var value FlexibleString
	require.NoError(t, value.UnmarshalJSON([]byte(`"100.25"`)))
	require.Equal(t, FlexibleString("100.25"), value)
	require.NoError(t, value.UnmarshalJSON([]byte(`100.25`)))
	require.Equal(t, FlexibleString("100.25"), value)
}
