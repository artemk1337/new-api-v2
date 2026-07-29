package model

import (
	"errors"

	"gorm.io/gorm"
)

const SystemTelemetryRetentionSeconds int64 = 24 * 60 * 60

// SystemTelemetrySample is a compact host resource snapshot collected by the
// privileged telemetry agent. TopProcesses is JSON to keep each sample to one
// write regardless of the number of reported processes.
type SystemTelemetrySample struct {
	ID            int64   `json:"id" gorm:"primaryKey"`
	NodeName      string  `json:"node_name" gorm:"type:varchar(128);uniqueIndex:idx_system_telemetry_node_collected,priority:1"`
	CollectedAt   int64   `json:"collected_at" gorm:"bigint;index;uniqueIndex:idx_system_telemetry_node_collected,priority:2"`
	CPUUsage      float64 `json:"cpu_usage"`
	MemoryUsage   float64 `json:"memory_usage"`
	SwapUsedBytes uint64  `json:"swap_used_bytes"`
	SwapUsage     float64 `json:"swap_usage"`
	IOWait        float64 `json:"io_wait"`
	LoadAverage1  float64 `json:"load_average_1"`
	DiskUsage     float64 `json:"disk_usage"`
	TopProcesses  string  `json:"top_processes" gorm:"type:text"`
}

func (SystemTelemetrySample) TableName() string {
	return "system_telemetry_samples"
}

func CreateSystemTelemetrySample(sample *SystemTelemetrySample) error {
	return DB.Create(sample).Error
}

func ListSystemTelemetrySamples(nodeName string, since int64) ([]SystemTelemetrySample, error) {
	var samples []SystemTelemetrySample
	err := DB.Where("node_name = ? AND collected_at >= ?", nodeName, since).
		Order("collected_at asc").
		Find(&samples).Error
	return samples, err
}

func DeleteSystemTelemetrySamplesBefore(cutoff int64) error {
	return DB.Where("collected_at < ?", cutoff).Delete(&SystemTelemetrySample{}).Error
}

func LatestSystemTelemetrySample(nodeName string) (*SystemTelemetrySample, error) {
	var sample SystemTelemetrySample
	err := DB.Where("node_name = ?", nodeName).Order("collected_at desc").First(&sample).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sample, nil
}
