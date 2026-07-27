package dto

type UpstreamDTO struct {
	ID       int    `json:"id,omitempty"`
	Name     string `json:"name" binding:"required"`
	BaseURL  string `json:"base_url" binding:"required"`
	Endpoint string `json:"endpoint"`
}

type UpstreamRequest struct {
	ChannelIDs []int64       `json:"channel_ids"`
	Upstreams  []UpstreamDTO `json:"upstreams"`
	Timeout    int           `json:"timeout"`
}

// TestResult 上游测试连通性结果
type TestResult struct {
	Name     string   `json:"name"`
	Status   string   `json:"status"`
	Error    string   `json:"error,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// DifferenceItem 差异项
// Current 为本地值，可能为 nil
// Upstreams 为各渠道的上游值，具体数值 / "same" / nil

type DifferenceItem struct {
	Current    interface{}            `json:"current"`
	Upstreams  map[string]interface{} `json:"upstreams"`
	Confidence map[string]bool        `json:"confidence"`
}

type SyncableChannel struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	Status  int    `json:"status"`
	Type    int    `json:"type"`
}

type PricingSyncSource struct {
	ChannelID       int    `json:"channel_id"`
	Enabled         bool   `json:"enabled"`
	Endpoint        string `json:"endpoint"`
	IntervalSeconds int    `json:"interval_seconds"`
}

type PricingSyncConfigRequest struct {
	Strategy        string              `json:"strategy"`
	Sources         []PricingSyncSource `json:"sources"`
	ExpectedVersion *int64              `json:"expected_version"`
}

type PricingSyncModelPreferenceRequest struct {
	ModelName string `json:"model_name"`
	Mode      string `json:"mode"`
	ChannelID int    `json:"channel_id"`
}
