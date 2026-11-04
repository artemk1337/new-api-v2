package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

// TaskMediaProxy serves only assets from an owned, persisted task result.
// It never accepts a URL or forwards a channel credential to an asset host.
func TaskMediaProxy(c *gin.Context) {
	task, exists, err := model.GetByTaskId(c.GetInt("id"), c.Param("task_id"))
	if err != nil {
		videoProxyError(c, http.StatusInternalServerError, "server_error", "Failed to query task")
		return
	}
	if !exists || task.Platform != constant.TaskPlatformSuno {
		videoProxyError(c, http.StatusNotFound, "invalid_request_error", "Task not found")
		return
	}
	index, err := strconv.Atoi(c.DefaultQuery("index", "0"))
	if err != nil || index < 0 {
		videoProxyError(c, http.StatusBadRequest, "invalid_request_error", "Invalid asset index")
		return
	}
	songs, err := task.SunoSongs()
	if err != nil || index >= len(songs) {
		videoProxyError(c, http.StatusNotFound, "invalid_request_error", "Asset not found")
		return
	}
	song := songs[index]
	assetURL := ""
	switch c.Query("kind") {
	case "audio":
		assetURL = song.AudioURL
	case "video":
		assetURL = song.VideoURL
	case "image":
		assetURL = song.ImageURL
	case "image-large":
		assetURL = song.ImageLargeURL
	}
	if strings.TrimSpace(assetURL) == "" {
		videoProxyError(c, http.StatusNotFound, "invalid_request_error", "Asset not found")
		return
	}
	proxy := ""
	if transport := task.PrivateData.ResultTransport; transport != nil {
		proxy = transport.Proxy
	}
	c.Set("supplier_pricing", true)
	proxyTaskPublicAsset(c, assetURL, proxy)
}

func validateTaskAssetURL(target string) error {
	setting := system_setting.GetFetchSetting()
	return common.ValidateURLWithFetchSetting(target, setting.EnableSSRFProtection, setting.AllowPrivateIp, setting.DomainFilterMode, setting.IpFilterMode, setting.DomainList, setting.IpList, setting.AllowedPorts, setting.ApplyIPFilterForDomain)
}

func proxyTaskPublicAsset(c *gin.Context, target, proxy string) {
	if err := validateTaskAssetURL(target); err != nil {
		videoProxyError(c, http.StatusForbidden, "server_error", "Asset URL is unavailable")
		return
	}
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		videoProxyError(c, http.StatusInternalServerError, "server_error", "Failed to create proxy client")
		return
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, target, nil)
	if err != nil {
		videoProxyError(c, http.StatusBadGateway, "server_error", "Asset URL is unavailable")
		return
	}
	if req.URL.Scheme != "https" && req.URL.Scheme != "http" || req.URL.User != nil {
		videoProxyError(c, http.StatusForbidden, "server_error", "Asset URL is unavailable")
		return
	}
	forwardVideoRequestHeaders(c.Request.Header, req.Header)
	assetClient := *client
	assetClient.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 10 || next.URL.User != nil {
			return fmt.Errorf("asset redirect is unavailable")
		}
		if err := validateTaskAssetURL(next.URL.String()); err != nil {
			return fmt.Errorf("asset redirect is unavailable")
		}
		return nil
	}
	resp, err := assetClient.Do(req)
	if err != nil {
		videoProxyError(c, http.StatusBadGateway, "server_error", "Failed to fetch asset")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		videoProxyError(c, http.StatusBadGateway, "server_error", "Failed to fetch asset")
		return
	}
	c.Set("supplier_pricing", true)
	if err := writeVideoResponse(c, resp); err != nil {
		common.SysError("failed to stream task asset")
	}
}

func taskMediaResponseHeader(key string) bool {
	switch strings.ToLower(key) {
	case "content-type", "content-length", "content-range", "accept-ranges", "etag", "last-modified", "cache-control", "expires":
		return true
	default:
		return false
	}
}
