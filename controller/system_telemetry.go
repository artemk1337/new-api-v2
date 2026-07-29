package controller

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

const (
	defaultTelemetryHours      = 24
	defaultTelemetrySidecarURL = "http://new-api-updater:18090"
)

var systemTelemetryHTTPClient = &http.Client{Timeout: 15 * time.Second}

type systemTelemetryProcess struct {
	PID      int     `json:"pid"`
	Name     string  `json:"name"`
	CPUUsage float64 `json:"cpu_usage"`
	RSSBytes uint64  `json:"rss_bytes"`
}

type systemTelemetryResponse struct {
	NodeName    string                   `json:"node_name"`
	CollectedAt int64                    `json:"collected_at"`
	CPUUsage    float64                  `json:"cpu_usage"`
	MemoryUsage float64                  `json:"memory_usage"`
	SwapUsed    uint64                   `json:"swap_used_bytes"`
	SwapUsage   float64                  `json:"swap_usage"`
	IOWait      float64                  `json:"io_wait"`
	LoadAverage float64                  `json:"load_average_1"`
	DiskUsage   float64                  `json:"disk_usage"`
	Top         []systemTelemetryProcess `json:"top_processes"`
}

func GetSystemTelemetry(c *gin.Context) {
	nodeName := strings.TrimSpace(c.Query("node_name"))
	if nodeName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "node_name is required"})
		return
	}
	hours := defaultTelemetryHours
	if rawHours := c.Query("hours"); rawHours != "" {
		parsed, err := strconv.Atoi(rawHours)
		if err != nil || (parsed != 1 && parsed != 6 && parsed != 24) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "hours must be 1, 6, or 24"})
			return
		}
		hours = parsed
	}

	samples, err := model.ListSystemTelemetrySamples(nodeName, time.Now().Add(-time.Duration(hours)*time.Hour).Unix())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result := make([]systemTelemetryResponse, 0, len(samples))
	for _, sample := range samples {
		item := systemTelemetryResponse{NodeName: sample.NodeName, CollectedAt: sample.CollectedAt, CPUUsage: sample.CPUUsage, MemoryUsage: sample.MemoryUsage, SwapUsed: sample.SwapUsedBytes, SwapUsage: sample.SwapUsage, IOWait: sample.IOWait, LoadAverage: sample.LoadAverage1, DiskUsage: sample.DiskUsage, Top: []systemTelemetryProcess{}}
		_ = common.UnmarshalJsonStr(sample.TopProcesses, &item.Top)
		if item.Top == nil {
			item.Top = []systemTelemetryProcess{}
		}
		result = append(result, item)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": result})
}

func GetSystemTelemetryAgent(c *gin.Context)   { proxySystemTelemetryAgent(c, http.MethodGet) }
func StartSystemTelemetryAgent(c *gin.Context) { proxySystemTelemetryAgent(c, http.MethodPost) }
func StopSystemTelemetryAgent(c *gin.Context)  { proxySystemTelemetryAgent(c, http.MethodDelete) }

func proxySystemTelemetryAgent(c *gin.Context, method string) {
	req, err := http.NewRequestWithContext(c.Request.Context(), method, telemetryAgentURL(), nil)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	response, err := systemTelemetryHTTPClient.Do(req)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "telemetry agent controller is unavailable"})
		return
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !jsonResponse(c, response.StatusCode, body) {
		c.JSON(response.StatusCode, gin.H{"success": false, "message": "invalid telemetry agent response"})
	}
}

func telemetryAgentURL() string {
	baseURL := common.GetEnvOrDefaultString("UPDATE_SIDECAR_URL", defaultTelemetrySidecarURL)
	return strings.TrimRight(baseURL, "/") + "/telemetry-agent"
}

func jsonResponse(c *gin.Context, status int, body []byte) bool {
	var payload any
	if common.Unmarshal(body, &payload) != nil {
		return false
	}
	c.Data(status, "application/json", bytes.TrimSpace(body))
	return true
}
