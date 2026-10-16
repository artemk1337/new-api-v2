package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

const dockerCacheResponseLimit = 16 << 20

var (
	dockerCachePruning bool // Guarded by updateMu, like updating.
	dockerCacheClient  = &http.Client{
		Timeout: 2 * time.Minute,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", "/var/run/docker.sock")
			},
			MaxIdleConns:        2,
			IdleConnTimeout:     30 * time.Second,
			MaxIdleConnsPerHost: 2,
		},
	}
)

func handleDockerBuildCache(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		var usage struct {
			BuildCacheUsage *struct {
				TotalSize   int64
				Reclaimable int64
				TotalCount  int
			}
			BuildCache []struct {
				Size   int64
				InUse  bool
				Shared bool
			}
		}
		if err := dockerCacheRequest(ctx, http.MethodGet, "/system/df?type=build-cache", &usage); err != nil {
			writeAnyJSON(w, http.StatusBadGateway, map[string]string{"message": err.Error()})
			return
		}
		if summary := usage.BuildCacheUsage; summary != nil {
			writeAnyJSON(w, http.StatusOK, dto.DockerBuildCacheStats{
				TotalSize:       summary.TotalSize,
				ReclaimableSize: summary.Reclaimable,
				RecordCount:     summary.TotalCount,
			})
			return
		}
		stats := dto.DockerBuildCacheStats{RecordCount: len(usage.BuildCache)}
		for _, record := range usage.BuildCache {
			stats.TotalSize += record.Size
			if !record.InUse && !record.Shared {
				stats.ReclaimableSize += record.Size
			}
		}
		writeAnyJSON(w, http.StatusOK, stats)
	case http.MethodDelete:
		updateMu.Lock()
		if updating || dockerCachePruning {
			updateMu.Unlock()
			writeAnyJSON(w, http.StatusConflict, map[string]string{"message": "update or Docker build cache cleanup is already running"})
			return
		}
		dockerCachePruning = true
		updateMu.Unlock()
		defer func() {
			updateMu.Lock()
			dockerCachePruning = false
			updateMu.Unlock()
		}()
		filters := url.Values{
			"all":     {"true"},
			"filters": {`{"until":["24h"]}`},
		}
		var result struct {
			SpaceReclaimed uint64
		}
		if err := dockerCacheRequest(r.Context(), http.MethodPost, "/build/prune?"+filters.Encode(), &result); err != nil {
			writeAnyJSON(w, http.StatusBadGateway, map[string]string{"message": err.Error()})
			return
		}
		writeAnyJSON(w, http.StatusOK, dto.DockerBuildCachePruneResult{SpaceReclaimed: result.SpaceReclaimed})
	default:
		w.Header().Set("Allow", "GET, DELETE")
		writeAnyJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
	}
}

func dockerCacheRequest(ctx context.Context, method, path string, result any) error {
	request, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, nil)
	if err != nil {
		return err
	}
	response, err := dockerCacheClient.Do(request)
	if err != nil {
		return fmt.Errorf("Docker build cache request failed: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, dockerCacheResponseLimit+1))
	if err != nil {
		return fmt.Errorf("read Docker build cache response: %w", err)
	}
	if len(data) > dockerCacheResponseLimit {
		return fmt.Errorf("Docker build cache response exceeds size limit")
	}
	if response.StatusCode != http.StatusOK {
		var failure struct {
			Message string `json:"message"`
		}
		if common.Unmarshal(data, &failure) == nil && failure.Message != "" {
			return fmt.Errorf("Docker build cache request failed: %s", failure.Message)
		}
		return fmt.Errorf("Docker build cache request failed with status %d", response.StatusCode)
	}
	if err := common.Unmarshal(data, result); err != nil {
		return fmt.Errorf("decode Docker build cache response: %w", err)
	}
	return nil
}
