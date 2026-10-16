package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

const dockerCacheUnavailableMessage = "Docker cache management is unavailable. Update and start the updater sidecar."

var dockerCacheHTTPClient = &http.Client{Timeout: 135 * time.Second}

func GetDockerBuildCache(ctx context.Context) (dto.DockerBuildCacheStats, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var stats dto.DockerBuildCacheStats
	err := requestDockerBuildCache(ctx, http.MethodGet, &stats)
	return stats, err
}

func ClearDockerBuildCache(ctx context.Context) (dto.DockerBuildCachePruneResult, error) {
	var result dto.DockerBuildCachePruneResult
	err := requestDockerBuildCache(ctx, http.MethodDelete, &result)
	return result, err
}

func requestDockerBuildCache(ctx context.Context, method string, result any) error {
	endpoint := strings.TrimRight(systemUpdateSidecarURL(), "/") + "/docker/build-cache"
	request, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := dockerCacheHTTPClient.Do(request)
	if err != nil {
		return errors.New(dockerCacheUnavailableMessage)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return errors.New(dockerCacheUnavailableMessage)
	}
	const responseLimit = 64 << 10
	body, err := io.ReadAll(io.LimitReader(response.Body, responseLimit+1))
	if err != nil {
		return fmt.Errorf("read Docker cache response: %w", err)
	}
	if len(body) > responseLimit {
		return errors.New("Docker cache response exceeds size limit")
	}
	if response.StatusCode != http.StatusOK {
		var failure struct {
			Message string `json:"message"`
		}
		if common.Unmarshal(body, &failure) == nil && failure.Message != "" {
			return errors.New(failure.Message)
		}
		return fmt.Errorf("Docker cache request failed with status %d", response.StatusCode)
	}
	return common.Unmarshal(body, result)
}
