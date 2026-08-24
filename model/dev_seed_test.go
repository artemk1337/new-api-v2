package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSeedDevDataRequiresDevBuildMarker(t *testing.T) {
	t.Setenv("NEW_API_DEV_SEED", "true")
	// Normal test/production builds do not carry the Dockerfile.dev linker
	// marker, so the seed must be a no-op even when the runtime env leaks in.
	require.NoError(t, SeedDevData())
}
