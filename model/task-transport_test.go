package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTaskTransportFingerprintCoversWholePoolAndRouting(t *testing.T) {
	baseURL := "https://source.example"
	channel := Channel{Id: 4, Type: 1, BaseURL: &baseURL, Key: "first-key\nsecond-key"}
	fingerprint := channel.TaskTransportFingerprint()
	task := &Task{ChannelId: 4, PrivateData: TaskPrivateData{TransportFingerprint: fingerprint}}
	require.NoError(t, ValidateTaskChannelTransport(task, &channel))
	require.Len(t, fingerprint, 64)
	require.NotContains(t, fingerprint, "first-key")
	channel.Keys = []string{"first-key"}
	require.Equal(t, fingerprint, channel.TaskTransportFingerprint())
	channel.Key = "first-key\nchanged-second-key"
	require.ErrorIs(t, ValidateTaskChannelTransport(task, &channel), ErrTaskChannelTransportChanged)
	channel.Key = "first-key\nsecond-key"
	channel.Type++
	require.ErrorIs(t, ValidateTaskChannelTransport(task, &channel), ErrTaskChannelTransportChanged)
}

func TestTaskTransportPairGuardPreservesLegacyNullBinding(t *testing.T) {
	supplierID, profileID := 7, 8
	channel := &Channel{Id: 1}
	legacy := &Task{ChannelId: 1}
	require.NoError(t, ValidateTaskChannelTransport(legacy, channel))
	channel.SupplierID, channel.SupplierProfileID = &supplierID, &profileID
	require.ErrorIs(t, ValidateTaskChannelTransport(legacy, channel), ErrTaskChannelTransportChanged)
	task := &Task{ChannelId: 1, PrivateData: TaskPrivateData{BillingContext: &TaskBillingContext{SupplierID: 7, ProfileID: 8}}}
	require.NoError(t, ValidateTaskChannelTransport(task, channel))
	changedProfile := 9
	channel.SupplierProfileID = &changedProfile
	require.ErrorIs(t, ValidateTaskChannelTransport(task, channel), ErrTaskChannelTransportChanged)
}
