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

const storageCleanupUnavailableMessage = "Server storage cleanup is unavailable. Update and start the updater sidecar."

var storageCleanupHTTPClient = &http.Client{Timeout: 135 * time.Second}

func GetDockerImageStats(ctx context.Context) (dto.DockerImageStats, error) {
	var stats dto.DockerImageStats
	err := requestStorageCleanup(ctx, http.MethodGet, "/docker/storage/images", &stats)
	return stats, err
}

func ClearDockerImages(ctx context.Context) (dto.DockerImagePruneResult, error) {
	var result dto.DockerImagePruneResult
	err := requestStorageCleanup(ctx, http.MethodDelete, "/docker/storage/images", &result)
	return result, err
}

func GetBackupStats(ctx context.Context) (dto.BackupStats, error) {
	var stats dto.BackupStats
	err := requestStorageCleanup(ctx, http.MethodGet, "/docker/storage/backups", &stats)
	return stats, err
}

func ClearOldBackups(ctx context.Context) (dto.BackupPruneResult, error) {
	var result dto.BackupPruneResult
	err := requestStorageCleanup(ctx, http.MethodDelete, "/docker/storage/backups", &result)
	return result, err
}

func requestStorageCleanup(ctx context.Context, method, path string, result any) error {
	timeout := 20 * time.Second
	if method == http.MethodDelete {
		timeout = 130 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	endpoint := strings.TrimRight(systemUpdateSidecarURL(), "/") + path
	request, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := storageCleanupHTTPClient.Do(request)
	if err != nil {
		return errors.New(storageCleanupUnavailableMessage)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return errors.New(storageCleanupUnavailableMessage)
	}
	const responseLimit = 64 << 10
	body, err := io.ReadAll(io.LimitReader(response.Body, responseLimit+1))
	if err != nil {
		return fmt.Errorf("read server storage response: %w", err)
	}
	if len(body) > responseLimit {
		return errors.New("server storage response exceeds size limit")
	}
	if response.StatusCode != http.StatusOK {
		var failure struct {
			Message string `json:"message"`
		}
		if common.Unmarshal(body, &failure) == nil && failure.Message != "" {
			return errors.New(failure.Message)
		}
		return fmt.Errorf("server storage request failed with status %d", response.StatusCode)
	}
	return common.Unmarshal(body, result)
}
