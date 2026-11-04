package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
)

var ErrTaskChannelTransportChanged = errors.New("task channel is unavailable")

// TaskTransportFingerprint identifies the full credential pool and transport,
// without persisting another copy of an upstream credential.
func (channel *Channel) TaskTransportFingerprint() string {
	baseURL := channel.GetBaseURL()
	if baseURL == "" && channel.Type >= 0 && channel.Type < len(constant.ChannelBaseURLs) {
		baseURL = constant.ChannelBaseURLs[channel.Type]
	}
	settings := dto.ChannelSettings{}
	if channel.Setting != nil {
		_ = common.UnmarshalJsonStr(*channel.Setting, &settings)
	}
	data, _ := common.Marshal(struct {
		Type    int
		BaseURL string
		KeyPool string
		Proxy   string
		Other   string
	}{channel.Type, baseURL, channel.Key, settings.Proxy, channel.Other})
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func ValidateTaskChannelTransport(task *Task, channel *Channel) error {
	if channel.Id != task.ChannelId {
		return ErrTaskChannelTransportChanged
	}
	supplierID, profileID := 0, 0
	if context := task.PrivateData.BillingContext; context != nil {
		supplierID, profileID = context.SupplierID, context.ProfileID
	}
	if supplierID == 0 && profileID == 0 {
		if channel.SupplierID != nil || channel.SupplierProfileID != nil {
			return ErrTaskChannelTransportChanged
		}
	} else if supplierID <= 0 || profileID <= 0 || channel.SupplierID == nil || channel.SupplierProfileID == nil ||
		*channel.SupplierID != supplierID || *channel.SupplierProfileID != profileID {
		return ErrTaskChannelTransportChanged
	}
	if fingerprint := task.PrivateData.TransportFingerprint; fingerprint != "" && fingerprint != channel.TaskTransportFingerprint() {
		return ErrTaskChannelTransportChanged
	}
	return nil
}

// GetTaskChannel bypasses the channel cache before any stored upstream task ID
// or credential is used. A changed task stays unresolved for administrative review.
func GetTaskChannel(task *Task) (*Channel, error) {
	channel, err := GetChannelById(task.ChannelId, true)
	if err != nil {
		return nil, errors.New("task channel is unavailable")
	}
	if err := ValidateTaskChannelTransport(task, channel); err != nil {
		return nil, err
	}
	return channel, nil
}
