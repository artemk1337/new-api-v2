package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackupCleanupRetainsTwoNewestEntriesAndWorkingStorage(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, time.November, 8, 12, 0, 0, 0, time.UTC)
	writeBackup := func(name string, age time.Duration, content string) {
		t.Helper()
		path := filepath.Join(directory, name)
		require.NoError(t, os.WriteFile(path, []byte(content), 0600))
		require.NoError(t, os.Chtimes(path, now.Add(-age), now.Add(-age)))
	}
	writeBackup("latest.dump", 24*time.Hour, "latest")
	writeBackup("second-latest.dump", 31*24*time.Hour, "second")
	writeBackup("old-a.dump", 40*24*time.Hour, "old-a")
	writeBackup("old-b.dump", 60*24*time.Hour, "old-b")

	workingStorage := filepath.Join(t.TempDir(), "postgres-data")
	require.NoError(t, os.Mkdir(workingStorage, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(workingStorage, "data"), []byte("live data"), 0600))
	link := filepath.Join(directory, "external-storage")
	if err := os.Symlink(workingStorage, link); err == nil {
		require.NoError(t, os.Chtimes(link, now.Add(-60*24*time.Hour), now.Add(-60*24*time.Hour)))
	}

	stats, err := inspectBackups(directory, now)
	require.NoError(t, err)
	assert.Equal(t, 4, stats.EntryCount)
	assert.EqualValues(t, len("old-a")+len("old-b"), stats.ReclaimableSize)
	assert.Equal(t, 2, stats.ReclaimableCount)

	result, err := pruneBackups(directory, now)
	require.NoError(t, err)
	assert.EqualValues(t, len("old-a")+len("old-b"), result.SpaceReclaimed)
	assert.Equal(t, 2, result.RemovedCount)
	assert.FileExists(t, filepath.Join(directory, "latest.dump"))
	assert.FileExists(t, filepath.Join(directory, "second-latest.dump"))
	assert.NoFileExists(t, filepath.Join(directory, "old-a.dump"))
	assert.NoFileExists(t, filepath.Join(directory, "old-b.dump"))
	assert.FileExists(t, filepath.Join(workingStorage, "data"))
}

func TestReadBackupEntriesRejectsBackupDirectorySymlink(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "backups")
	require.NoError(t, os.Symlink(target, link))

	_, err := readBackupEntries(link)
	require.Error(t, err)
	assert.ErrorContains(t, err, "not a symlink")
}

type dockerStorageRoundTripper func(*http.Request) (*http.Response, error)

func (roundTripper dockerStorageRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTripper(request)
}

func TestDockerImageCleanupReportsOldUnusedImagesAndPrunesAllUnused(t *testing.T) {
	savedClient := dockerCacheClient
	savedUpdating, savedCachePruning, savedStoragePruning := updating, dockerCachePruning, dockerStoragePruning
	t.Cleanup(func() {
		dockerCacheClient = savedClient
		updating, dockerCachePruning, dockerStoragePruning = savedUpdating, savedCachePruning, savedStoragePruning
	})
	updating, dockerCachePruning, dockerStoragePruning = false, false, false
	requests := 0
	dockerCacheClient = &http.Client{Transport: dockerStorageRoundTripper(func(request *http.Request) (*http.Response, error) {
		requests++
		switch request.Method {
		case http.MethodGet:
			old := strconv.FormatInt(time.Now().Add(-40*24*time.Hour).Unix(), 10)
			recent := strconv.FormatInt(time.Now().Add(-24*time.Hour).Unix(), 10)
			body := `{"Images":[{"Created":` + old + `,"Size":100,"Containers":0},{"Created":` + old + `,"Size":200,"Containers":1},{"Created":` + recent + `,"Size":300,"Containers":0}]}`
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		case http.MethodPost:
			assert.Equal(t, "/images/prune", request.URL.Path)
			assert.Equal(t, "true", request.URL.Query().Get("all"))
			assert.Contains(t, request.URL.Query().Get("filters"), "720h")
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"SpaceReclaimed":123}`)), Header: make(http.Header)}, nil
		default:
			return nil, assert.AnError
		}
	})}

	statsResponse := httptest.NewRecorder()
	handleDockerImageCleanup(statsResponse, httptest.NewRequest(http.MethodGet, "/docker/storage/images", nil))
	require.Equal(t, http.StatusOK, statsResponse.Code)
	assert.JSONEq(t, `{"total_size":600,"reclaimable_size":100,"image_count":3,"reclaimable_count":1}`, statsResponse.Body.String())

	pruneResponse := httptest.NewRecorder()
	handleDockerImageCleanup(pruneResponse, httptest.NewRequest(http.MethodDelete, "/docker/storage/images", nil))
	require.Equal(t, http.StatusOK, pruneResponse.Code)
	assert.JSONEq(t, `{"space_reclaimed":123}`, pruneResponse.Body.String())
	assert.Equal(t, 2, requests)
}
