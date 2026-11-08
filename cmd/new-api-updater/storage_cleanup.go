package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

const (
	imageRetentionAge    = 30 * 24 * time.Hour
	backupRetentionAge   = 30 * 24 * time.Hour
	minimumBackupCount   = 2
	storageResponseLimit = 16 << 20
)

var dockerStoragePruning bool // Guarded by updateMu, like updating.

func handleDockerImageCleanup(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		var usage struct {
			Images []struct {
				Created    int64
				Size       int64
				Containers int
			} `json:"Images"`
		}
		if err := dockerStorageRequest(ctx, http.MethodGet, "/system/df?type=image", &usage); err != nil {
			writeAnyJSON(w, http.StatusBadGateway, map[string]string{"message": err.Error()})
			return
		}
		stats := dto.DockerImageStats{ImageCount: len(usage.Images)}
		cutoff := time.Now().Add(-imageRetentionAge).Unix()
		for _, image := range usage.Images {
			stats.TotalSize += image.Size
			if image.Containers == 0 && image.Created < cutoff {
				stats.ReclaimableSize += image.Size
				stats.ReclaimableCount++
			}
		}
		writeAnyJSON(w, http.StatusOK, stats)
	case http.MethodDelete:
		if !beginDockerStorageCleanup(w) {
			return
		}
		defer endDockerStorageCleanup()
		filters := url.Values{
			"all":     {"true"},
			"filters": {`{"until":["720h"]}`},
		}
		var result struct {
			SpaceReclaimed uint64 `json:"SpaceReclaimed"`
		}
		if err := dockerStorageRequest(r.Context(), http.MethodPost, "/images/prune?"+filters.Encode(), &result); err != nil {
			writeAnyJSON(w, http.StatusBadGateway, map[string]string{"message": err.Error()})
			return
		}
		writeAnyJSON(w, http.StatusOK, dto.DockerImagePruneResult{SpaceReclaimed: result.SpaceReclaimed})
	default:
		w.Header().Set("Allow", "GET, DELETE")
		writeAnyJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
	}
}

func handleBackupCleanup(w http.ResponseWriter, r *http.Request) {
	composeDir, err := updaterComposeDir()
	if err != nil {
		writeAnyJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	backupDir := filepath.Join(composeDir, "backups")
	switch r.Method {
	case http.MethodGet:
		stats, err := inspectBackups(backupDir, time.Now())
		if err != nil {
			writeAnyJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
			return
		}
		writeAnyJSON(w, http.StatusOK, stats)
	case http.MethodDelete:
		if !beginDockerStorageCleanup(w) {
			return
		}
		defer endDockerStorageCleanup()
		result, err := pruneBackups(backupDir, time.Now())
		if err != nil {
			writeAnyJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
			return
		}
		writeAnyJSON(w, http.StatusOK, result)
	default:
		w.Header().Set("Allow", "GET, DELETE")
		writeAnyJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
	}
}

func beginDockerStorageCleanup(w http.ResponseWriter) bool {
	updateMu.Lock()
	defer updateMu.Unlock()
	if updating || dockerCachePruning || dockerStoragePruning {
		writeAnyJSON(w, http.StatusConflict, map[string]string{"message": "update or storage cleanup is already running"})
		return false
	}
	dockerStoragePruning = true
	return true
}

func endDockerStorageCleanup() {
	updateMu.Lock()
	dockerStoragePruning = false
	updateMu.Unlock()
}

func dockerStorageRequest(ctx context.Context, method, path string, result any) error {
	request, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, nil)
	if err != nil {
		return err
	}
	response, err := dockerCacheClient.Do(request)
	if err != nil {
		return fmt.Errorf("Docker storage request failed: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, storageResponseLimit+1))
	if err != nil {
		return fmt.Errorf("read Docker storage response: %w", err)
	}
	if len(data) > storageResponseLimit {
		return errors.New("Docker storage response exceeds size limit")
	}
	if response.StatusCode != http.StatusOK {
		var failure struct {
			Message string `json:"message"`
		}
		if common.Unmarshal(data, &failure) == nil && failure.Message != "" {
			return fmt.Errorf("Docker storage request failed: %s", failure.Message)
		}
		return fmt.Errorf("Docker storage request failed with status %d", response.StatusCode)
	}
	return common.Unmarshal(data, result)
}

type backupEntry struct {
	path    string
	name    string
	modTime time.Time
	size    int64
}

func inspectBackups(directory string, now time.Time) (dto.BackupStats, error) {
	entries, err := readBackupEntries(directory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return dto.BackupStats{}, nil
		}
		return dto.BackupStats{}, err
	}
	stats := dto.BackupStats{EntryCount: len(entries)}
	for _, entry := range entries {
		stats.TotalSize += entry.size
	}
	for _, entry := range backupPruneCandidates(entries, now) {
		stats.ReclaimableSize += entry.size
		stats.ReclaimableCount++
	}
	return stats, nil
}

func pruneBackups(directory string, now time.Time) (dto.BackupPruneResult, error) {
	entries, err := readBackupEntries(directory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return dto.BackupPruneResult{}, nil
		}
		return dto.BackupPruneResult{}, err
	}
	result := dto.BackupPruneResult{}
	for _, entry := range backupPruneCandidates(entries, now) {
		if err := removeBackupEntry(entry.path); err != nil {
			return result, fmt.Errorf("remove backup %q: %w", entry.name, err)
		}
		result.SpaceReclaimed += entry.size
		result.RemovedCount++
	}
	return result, nil
}

func readBackupEntries(directory string) ([]backupEntry, error) {
	info, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("backup path must be a directory, not a symlink")
	}
	rootDevice, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, errors.New("unable to inspect backup filesystem")
	}
	parentInfo, err := os.Stat(filepath.Dir(directory))
	if err != nil {
		return nil, err
	}
	parentDevice, ok := parentInfo.Sys().(*syscall.Stat_t)
	if !ok || parentDevice.Dev != rootDevice.Dev {
		return nil, errors.New("backup directory is on a separate filesystem")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	result := make([]backupEntry, 0, len(entries))
	for _, entry := range entries {
		path := filepath.Join(directory, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		entryDevice, ok := info.Sys().(*syscall.Stat_t)
		if !ok || entryDevice.Dev != rootDevice.Dev {
			continue
		}
		size, err := backupEntrySize(path, info)
		if err != nil {
			return nil, err
		}
		result = append(result, backupEntry{path: path, name: entry.Name(), modTime: info.ModTime(), size: size})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].modTime.Equal(result[j].modTime) {
			return result[i].name > result[j].name
		}
		return result[i].modTime.After(result[j].modTime)
	})
	return result, nil
}

func backupEntrySize(path string, info os.FileInfo) (int64, error) {
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		if info.Mode().IsRegular() {
			return info.Size(), nil
		}
		return 0, nil
	}
	rootDevice, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("unable to inspect backup filesystem")
	}
	var size int64
	err := filepath.WalkDir(path, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current != path {
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if entry.IsDir() {
				child, err := entry.Info()
				if err != nil {
					return err
				}
				device, ok := child.Sys().(*syscall.Stat_t)
				if !ok || device.Dev != rootDevice.Dev {
					return filepath.SkipDir
				}
				return nil
			}
		}
		child, err := entry.Info()
		if err != nil {
			return err
		}
		if child.Mode().IsRegular() {
			device, ok := child.Sys().(*syscall.Stat_t)
			if ok && device.Dev == rootDevice.Dev {
				size += child.Size()
			}
		}
		return nil
	})
	return size, err
}

func removeBackupEntry(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return os.Remove(path)
	}
	rootDevice, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("unable to inspect backup filesystem")
	}
	var directories []string
	err = filepath.WalkDir(path, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		child, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if child.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		device, ok := child.Sys().(*syscall.Stat_t)
		if !ok || device.Dev != rootDevice.Dev {
			if child.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if child.IsDir() {
			directories = append(directories, current)
			return nil
		}
		if child.Mode().IsRegular() {
			return os.Remove(current)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(directories, func(i, j int) bool { return len(directories[i]) > len(directories[j]) })
	for _, directory := range directories {
		if err := os.Remove(directory); err != nil {
			return err
		}
	}
	return nil
}

func backupPruneCandidates(entries []backupEntry, now time.Time) []backupEntry {
	cutoff := now.Add(-backupRetentionAge)
	result := make([]backupEntry, 0)
	for index, entry := range entries {
		if index >= minimumBackupCount && entry.modTime.Before(cutoff) {
			result = append(result, entry)
		}
	}
	return result
}
