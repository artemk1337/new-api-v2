package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSystemTelemetryRetentionAndQuery(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&SystemTelemetrySample{}))
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })

	require.NoError(t, CreateSystemTelemetrySample(&SystemTelemetrySample{NodeName: "node-a", CollectedAt: 100, CPUUsage: 10}))
	require.NoError(t, CreateSystemTelemetrySample(&SystemTelemetrySample{NodeName: "node-a", CollectedAt: 200, CPUUsage: 20}))
	require.NoError(t, CreateSystemTelemetrySample(&SystemTelemetrySample{NodeName: "node-b", CollectedAt: 200, CPUUsage: 30}))

	samples, err := ListSystemTelemetrySamples("node-a", 150)
	require.NoError(t, err)
	require.Len(t, samples, 1)
	require.Equal(t, int64(200), samples[0].CollectedAt)

	require.NoError(t, DeleteSystemTelemetrySamplesBefore(200))
	samples, err = ListSystemTelemetrySamples("node-a", 0)
	require.NoError(t, err)
	require.Len(t, samples, 1)
}
