package main

import (
	"errors"
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

func TestValidateRequestAllowsOnlyConfiguredServiceAndSafeTag(t *testing.T) {
	require.NoError(t, validateRequest(updateRequest{
		Tag: "v1.2.3",
	}))
	require.Error(t, validateRequest(updateRequest{
		Tag: "v1.2.3;rm",
	}))
}

func TestNormalizeRepositoryURL(t *testing.T) {
	assert.Equal(t, "https://github.com/artemk1337/new-api-v2.git", normalizeRepositoryURL("artemk1337/new-api-v2"))
	assert.Equal(t, "https://example.com/repo.git", normalizeRepositoryURL("https://example.com/repo.git"))
}

func TestUpsertEnvFileUpdatesImageAndVersion(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(envFile, []byte("KEEP=value\nNEW_API_VERSION=old\n"), 0644))

	require.NoError(t, upsertEnvFile(envFile, map[string]string{
		"NEW_API_IMAGE":   "ghcr.io/artemk1337/new-api-v2",
		"NEW_API_VERSION": "v1.2.3",
	}))

	data, err := os.ReadFile(envFile)
	require.NoError(t, err)
	assert.Equal(t, "KEEP=value\nNEW_API_VERSION=v1.2.3\nNEW_API_IMAGE=ghcr.io/artemk1337/new-api-v2\n", string(data))
}

func TestUpsertEnvFilePreservesComments(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(envFile, []byte("# deploy settings\nKEEP=value\n\n"), 0644))

	require.NoError(t, upsertEnvFile(envFile, map[string]string{
		"NEW_API_VERSION": "v1.2.3",
	}))

	data, err := os.ReadFile(envFile)
	require.NoError(t, err)
	assert.Equal(t, "# deploy settings\nKEEP=value\n\nNEW_API_VERSION=v1.2.3\n", string(data))
}

func TestDeployPreparedImageRollsBackEnvOnComposeFailure(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	composeFile := filepath.Join(dir, "docker-compose.yml")
	require.NoError(t, os.WriteFile(envFile, []byte("KEEP=value\nNEW_API_IMAGE=old/image\nNEW_API_VERSION=v1.0.0\n"), 0644))
	require.NoError(t, os.WriteFile(composeFile, []byte("services: {}\n"), 0644))

	t.Setenv("UPDATER_COMPOSE_DIR", dir)
	t.Setenv("UPDATER_ENV_FILE", envFile)
	t.Setenv("UPDATER_COMPOSE_FILE", composeFile)
	t.Setenv("UPDATER_SERVICE", "new-api")
	t.Setenv("UPDATER_IMAGE", "local/new-api")

	var calls int
	saved := runCommandFn
	savedOutput := runCommandOutputFn
	runCommandFn = func(_ string, name string, args ...string) error {
		require.Equal(t, "docker", name)
		require.Contains(t, strings.Join(args, " "), "compose")
		calls++
		if calls == 1 {
			data, err := os.ReadFile(envFile)
			require.NoError(t, err)
			assert.Contains(t, string(data), "NEW_API_IMAGE=local/new-api\n")
			assert.Contains(t, string(data), "NEW_API_VERSION=v1.2.3\n")
			return errors.New("compose failed")
		}
		data, err := os.ReadFile(envFile)
		require.NoError(t, err)
		assert.Contains(t, string(data), "NEW_API_IMAGE=old/image\n")
		assert.Contains(t, string(data), "NEW_API_VERSION=v1.0.0\n")
		return nil
	}
	runCommandOutputFn = func(_ string, name string, args ...string) (string, error) {
		require.Equal(t, "docker", name)
		switch args[0] {
		case "exec":
			return `{"version":"v1.0.0"}`, nil
		case "inspect":
			return "healthy", nil
		default:
			t.Fatalf("unexpected docker command: %s", strings.Join(args, " "))
		}
		return "", nil
	}
	t.Cleanup(func() {
		runCommandFn = saved
		runCommandOutputFn = savedOutput
	})

	err := deployPreparedImage("v1.2.3")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compose failed")
	assert.Equal(t, 2, calls)

	data, err := os.ReadFile(envFile)
	require.NoError(t, err)
	assert.Equal(t, "KEEP=value\nNEW_API_IMAGE=old/image\nNEW_API_VERSION=v1.0.0\n", string(data))
}

func TestDeployPreparedImageRemovesInsertedEnvKeysOnRollback(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	composeFile := filepath.Join(dir, "docker-compose.yml")
	require.NoError(t, os.WriteFile(envFile, []byte("KEEP=value\n"), 0644))
	require.NoError(t, os.WriteFile(composeFile, []byte("services: {}\n"), 0644))

	t.Setenv("UPDATER_COMPOSE_DIR", dir)
	t.Setenv("UPDATER_ENV_FILE", envFile)
	t.Setenv("UPDATER_COMPOSE_FILE", composeFile)
	t.Setenv("UPDATER_SERVICE", "new-api")
	t.Setenv("UPDATER_IMAGE", "local/new-api")

	var calls int
	saved := runCommandFn
	savedOutput := runCommandOutputFn
	runCommandFn = func(_ string, name string, args ...string) error {
		require.Equal(t, "docker", name)
		require.Contains(t, strings.Join(args, " "), "compose")
		calls++
		if calls == 1 {
			return errors.New("compose failed")
		}
		return nil
	}
	runCommandOutputFn = func(_ string, name string, args ...string) (string, error) {
		require.Equal(t, "docker", name)
		switch args[0] {
		case "exec":
			return `{"version":"v1.0.0"}`, nil
		case "inspect":
			return "healthy", nil
		default:
			t.Fatalf("unexpected docker command: %s", strings.Join(args, " "))
		}
		return "", nil
	}
	t.Cleanup(func() {
		runCommandFn = saved
		runCommandOutputFn = savedOutput
	})

	err := deployPreparedImage("v1.2.3")
	require.Error(t, err)
	assert.Equal(t, 2, calls)

	data, err := os.ReadFile(envFile)
	require.NoError(t, err)
	assert.Equal(t, "KEEP=value\n", string(data))
}

func TestDeployPreparedImageRollsBackWhenNewServiceNeverGetsHealthy(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	composeFile := filepath.Join(dir, "docker-compose.yml")
	require.NoError(t, os.WriteFile(envFile, []byte("KEEP=value\nNEW_API_IMAGE=old/image\nNEW_API_VERSION=v1.0.0\n"), 0644))
	require.NoError(t, os.WriteFile(composeFile, []byte("services: {}\n"), 0644))

	t.Setenv("UPDATER_COMPOSE_DIR", dir)
	t.Setenv("UPDATER_ENV_FILE", envFile)
	t.Setenv("UPDATER_COMPOSE_FILE", composeFile)
	t.Setenv("UPDATER_SERVICE", "new-api")
	t.Setenv("UPDATER_IMAGE", "local/new-api")

	var composeCalls int
	var inspectCalls int
	saved := runCommandFn
	savedOutput := runCommandOutputFn
	runCommandFn = func(_ string, name string, args ...string) error {
		require.Equal(t, "docker", name)
		require.Contains(t, strings.Join(args, " "), "compose")
		composeCalls++
		return nil
	}
	runCommandOutputFn = func(_ string, name string, args ...string) (string, error) {
		require.Equal(t, "docker", name)
		switch args[0] {
		case "inspect":
			inspectCalls++
			if inspectCalls == 1 {
				return "unhealthy", nil
			}
			return "healthy", nil
		case "exec":
			return `{"version":"v1.0.0"}`, nil
		default:
			t.Fatalf("unexpected docker command: %s", strings.Join(args, " "))
		}
		return "", nil
	}
	t.Cleanup(func() {
		runCommandFn = saved
		runCommandOutputFn = savedOutput
	})

	savedTimeout := deployHealthTimeout
	savedInterval := deployHealthInterval
	deployHealthTimeout = time.Millisecond
	deployHealthInterval = time.Millisecond
	t.Cleanup(func() {
		deployHealthTimeout = savedTimeout
		deployHealthInterval = savedInterval
	})

	err := deployPreparedImage("v1.2.3")
	require.Error(t, err)
	assert.Equal(t, 2, composeCalls)

	data, err := os.ReadFile(envFile)
	require.NoError(t, err)
	assert.Equal(t, "KEEP=value\nNEW_API_IMAGE=old/image\nNEW_API_VERSION=v1.0.0\n", string(data))
}

func TestDeployPreparedImagePersistsUpdaterEnv(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	composeFile := filepath.Join(dir, "docker-compose.yml")
	require.NoError(t, os.WriteFile(envFile, []byte("KEEP=value\n"), 0644))
	require.NoError(t, os.WriteFile(composeFile, []byte("services: {}\n"), 0644))

	t.Setenv("UPDATER_COMPOSE_DIR", dir)
	t.Setenv("UPDATER_ENV_FILE", envFile)
	t.Setenv("UPDATER_COMPOSE_FILE", composeFile)
	t.Setenv("UPDATER_SERVICE", "new-api")
	t.Setenv("UPDATER_IMAGE", "local/new-api")
	t.Setenv("UPDATER_SHARED_SECRET", "secret")

	saved := runCommandFn
	savedOutput := runCommandOutputFn
	runCommandFn = func(_ string, name string, args ...string) error {
		require.Equal(t, "docker", name)
		require.Contains(t, strings.Join(args, " "), "compose")
		return nil
	}
	runCommandOutputFn = func(_ string, name string, args ...string) (string, error) {
		require.Equal(t, "docker", name)
		switch args[0] {
		case "exec":
			return `{"version":"v1.2.3"}`, nil
		case "inspect":
			return "healthy", nil
		default:
			t.Fatalf("unexpected docker command: %s", strings.Join(args, " "))
		}
		return "", nil
	}
	t.Cleanup(func() {
		runCommandFn = saved
		runCommandOutputFn = savedOutput
	})

	savedTimeout := deployHealthTimeout
	savedInterval := deployHealthInterval
	deployHealthTimeout = time.Millisecond
	deployHealthInterval = time.Millisecond
	t.Cleanup(func() {
		deployHealthTimeout = savedTimeout
		deployHealthInterval = savedInterval
	})

	require.NoError(t, deployPreparedImage("v1.2.3"))

	data, err := os.ReadFile(envFile)
	require.NoError(t, err)
	assert.Equal(t, "KEEP=value\nNEW_API_IMAGE=local/new-api\nNEW_API_VERSION=v1.2.3\nUPDATE_ENABLED=true\nUPDATE_SIDECAR_TOKEN=secret\n", string(data))
}

func TestSyncRepositoryUpdatesExistingRemoteURL(t *testing.T) {
	cacheDir := t.TempDir()
	repoDir := filepath.Join(cacheDir, "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, ".git"), 0755))
	t.Setenv("UPDATER_CACHE_DIR", cacheDir)
	t.Setenv("UPDATE_REPOSITORY", "artemk1337/new-api-v2")

	var commands []string
	saved := runCommandFn
	runCommandFn = func(dir string, name string, args ...string) error {
		commands = append(commands, dir+" "+name+" "+strings.Join(args, " "))
		return nil
	}
	t.Cleanup(func() {
		runCommandFn = saved
	})

	got, err := syncRepository()
	require.NoError(t, err)
	assert.Equal(t, repoDir, got)
	require.Len(t, commands, 2)
	assert.Contains(t, commands[0], "git remote set-url origin https://github.com/artemk1337/new-api-v2.git")
	assert.Contains(t, commands[1], "git fetch --tags --force origin")
}

func TestExtractStatusVersion(t *testing.T) {
	assert.Equal(t, "v1.2.3", extractStatusVersion(`{"success":true,"version":"v1.2.3"}`))
	assert.Empty(t, extractStatusVersion(`{"success":true}`))
}

func TestHandleJobStatusWritesSnapshot(t *testing.T) {
	t.Setenv("UPDATER_SHARED_SECRET", "secret")
	savedJobs := jobs
	jobs = map[string]*updateJob{}
	t.Cleanup(func() {
		jobs = savedJobs
	})

	jobs["update_1"] = &updateJob{
		JobID:   "update_1",
		Status:  "running",
		Step:    "building",
		Message: "building update image",
	}

	req := httptest.NewRequest(http.MethodGet, "/jobs/update_1", nil)
	req.Header.Set("Authorization", "Bearer secret")
	recorder := httptest.NewRecorder()

	handleJobStatus(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"job_id":"update_1","status":"running","step":"building","message":"building update image"}`, recorder.Body.String())
}

func TestHandleJobStatusRejectsUnauthorizedRequest(t *testing.T) {
	t.Setenv("UPDATER_SHARED_SECRET", "secret")

	req := httptest.NewRequest(http.MethodGet, "/jobs/update_1", nil)
	recorder := httptest.NewRecorder()

	handleJobStatus(recorder, req)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.JSONEq(t, `{"accepted":false,"message":"unauthorized"}`, recorder.Body.String())
}

func TestHandleUpdateRejectsMissingSharedSecret(t *testing.T) {
	t.Setenv("UPDATER_SHARED_SECRET", "")

	req := httptest.NewRequest(http.MethodPost, "/update", strings.NewReader(`{"tag":"v1.2.3"}`))
	recorder := httptest.NewRecorder()

	handleUpdate(recorder, req)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.JSONEq(t, `{"accepted":false,"message":"updater shared secret is not configured"}`, recorder.Body.String())
}

func TestCleanupOldJobsKeepsRecentTerminalJobs(t *testing.T) {
	savedJobs := jobs
	savedRetention := jobRetention
	jobs = map[string]*updateJob{}
	jobRetention = time.Hour
	t.Cleanup(func() {
		jobs = savedJobs
		jobRetention = savedRetention
	})

	now := time.Unix(100, 0)
	oldID := "update_" + strconv.FormatInt(now.Add(-2*time.Hour).UnixNano(), 10)
	recentID := "update_" + strconv.FormatInt(now.Add(-30*time.Minute).UnixNano(), 10)
	runningID := "update_" + strconv.FormatInt(now.Add(-2*time.Hour).UnixNano(), 10) + "_running"
	jobs[oldID] = &updateJob{JobID: oldID, Status: "failed"}
	jobs[recentID] = &updateJob{JobID: recentID, Status: "succeeded"}
	jobs[runningID] = &updateJob{JobID: runningID, Status: "running"}

	cleanupOldJobs(now)

	assert.NotContains(t, jobs, oldID)
	assert.Contains(t, jobs, recentID)
	assert.Contains(t, jobs, runningID)
}
