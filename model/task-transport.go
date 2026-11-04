package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
)

var ErrTaskChannelTransportChanged = errors.New("task channel is unavailable")

// TaskResultTransport retains the destination of an accepted job. It is used
// only to retrieve completed output; active jobs still validate the live channel.
type TaskResultTransport struct {
	Type    int    `json:"type"`
	BaseURL string `json:"base_url"`
	Proxy   string `json:"proxy,omitempty"`
}

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
	if err := validateTaskChannelBinding(task, channel); err != nil {
		return err
	}
	if fingerprint := task.PrivateData.TransportFingerprint; fingerprint != "" && fingerprint != channel.TaskTransportFingerprint() {
		return ErrTaskChannelTransportChanged
	}
	return nil
}

func validateTaskChannelBinding(task *Task, channel *Channel) error {
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

func GetTaskResultChannel(task *Task) (*Channel, error) {
	transport := task.PrivateData.ResultTransport
	if task.Status != TaskStatusSuccess || transport == nil {
		return GetTaskChannel(task)
	}
	setting, err := common.Marshal(dto.ChannelSettings{Proxy: transport.Proxy})
	if err != nil {
		return nil, ErrTaskChannelTransportChanged
	}
	settingJSON := string(setting)
	baseURL := transport.BaseURL
	channel := &Channel{Id: task.ChannelId, Type: transport.Type, BaseURL: &baseURL, Key: task.PrivateData.Key, Setting: &settingJSON}
	if context := task.PrivateData.BillingContext; context != nil && context.SupplierID > 0 {
		channel.SupplierID, channel.SupplierProfileID = &context.SupplierID, &context.ProfileID
	}
	// A rotated key may still retrieve historical output, but only at the
	// identical original destination and supplier/profile. A replacement host
	// must never receive either the old key or a task ID from the old source.
	if current, currentErr := GetChannelById(task.ChannelId, true); currentErr == nil && taskResultDestinationMatches(task, current) {
		keyIndex := slices.Index(current.GetKeys(), task.PrivateData.Key)
		status, hasStatus := current.ChannelInfo.MultiKeyStatusList[keyIndex]
		if keyIndex < 0 || hasStatus && status != common.ChannelStatusEnabled {
			if key, _, keyErr := current.GetNextEnabledKey(); keyErr == nil {
				channel.Key = key
			}
		}
	}
	return channel, nil
}

func taskResultDestinationMatches(task *Task, channel *Channel) bool {
	transport := task.PrivateData.ResultTransport
	baseURL := channel.GetBaseURL()
	if baseURL == "" && channel.Type >= 0 && channel.Type < len(constant.ChannelBaseURLs) {
		baseURL = constant.ChannelBaseURLs[channel.Type]
	}
	return transport != nil && validateTaskChannelBinding(task, channel) == nil && channel.Type == transport.Type &&
		baseURL == transport.BaseURL && channel.GetSetting().Proxy == transport.Proxy
}

func ValidateTaskResultChannel(task *Task, channel *Channel) error {
	transport := task.PrivateData.ResultTransport
	if task.Status != TaskStatusSuccess || transport == nil {
		return ValidateTaskChannelTransport(task, channel)
	}
	if channel.Id != task.ChannelId || channel.Type != transport.Type || channel.GetBaseURL() != transport.BaseURL ||
		channel.GetSetting().Proxy != transport.Proxy {
		return ErrTaskChannelTransportChanged
	}
	if channel.Key != task.PrivateData.Key {
		current, err := GetChannelById(task.ChannelId, true)
		if err != nil || !taskResultDestinationMatches(task, current) || !slices.Contains(current.GetKeys(), channel.Key) {
			return ErrTaskChannelTransportChanged
		}
	}
	return nil
}
