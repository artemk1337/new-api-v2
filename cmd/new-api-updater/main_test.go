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
	t.Setenv("UPDATER_IMAGE", "ghcr.io/artemk1337/new-api-v2")

	var calls int
	saved := runCommandFn
	savedOutput := runCommandOutputFn
	runCommandFn = func(_ string, name string, args ...string) error {
		require.Equal(t, "docker", name)
		require.Contains(t, strings.Join(args, " "), "compose")
		assert.Contains(t, args, "-p")
		assert.Contains(t, args, "folder-independent-project")
		calls++
		if calls == 1 {
			data, err := os.ReadFile(envFile)
			require.NoError(t, err)
			assert.Contains(t, string(data), "NEW_API_IMAGE=ghcr.io/artemk1337/new-api-v2\n")
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
			if strings.Contains(strings.Join(args, " "), "com.docker.compose.project") {
				return "folder-independent-project", nil
			}
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
	t.Setenv("UPDATER_IMAGE", "ghcr.io/artemk1337/new-api-v2")
	t.Setenv("UPDATER_COMPOSE_PROJECT_NAME", "production-api")

	var calls int
	saved := runCommandFn
	savedOutput := runCommandOutputFn
	runCommandFn = func(_ string, name string, args ...string) error {
		require.Equal(t, "docker", name)
		require.Contains(t, strings.Join(args, " "), "compose")
		assert.Contains(t, args, "-p")
		assert.Contains(t, args, "production-api")
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
			if strings.Contains(strings.Join(args, " "), "com.docker.compose.project") {
				return "ignored-by-explicit-env", nil
			}
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
	t.Setenv("UPDATER_IMAGE", "ghcr.io/artemk1337/new-api-v2")

	var composeCalls int
	var inspectCalls int
	saved := runCommandFn
	savedOutput := runCommandOutputFn
	runCommandFn = func(_ string, name string, args ...string) error {
		require.Equal(t, "docker", name)
		require.Contains(t, strings.Join(args, " "), "compose")
		assert.Contains(t, args, "-p")
		assert.Contains(t, args, "folder-independent-project")
		composeCalls++
		return nil
	}
	runCommandOutputFn = func(_ string, name string, args ...string) (string, error) {
		require.Equal(t, "docker", name)
		switch args[0] {
		case "inspect":
			if strings.Contains(strings.Join(args, " "), "com.docker.compose.project") {
				return "folder-independent-project", nil
			}
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

func TestComposeProjectNameUsesEnvFileValueBeforeFallback(t *testing.T) {
	t.Setenv("UPDATER_COMPOSE_PROJECT_NAME", "")
	t.Setenv("COMPOSE_PROJECT_NAME", "")
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(envFile, []byte("COMPOSE_PROJECT_NAME=custom-api\n"), 0644))

	assert.Equal(t, "custom-api", composeProjectName(dir, envFile, "new-api"))
}

func TestComposeProjectNameUsesExistingContainerLabelBeforeFallback(t *testing.T) {
	t.Setenv("UPDATER_COMPOSE_PROJECT_NAME", "")
	t.Setenv("COMPOSE_PROJECT_NAME", "")

	savedOutput := runCommandOutputFn
	runCommandOutputFn = func(_ string, name string, args ...string) (string, error) {
		require.Equal(t, "docker", name)
		assert.Equal(t, []string{"inspect", "-f", `{{ index .Config.Labels "com.docker.compose.project" }}`, "new-api"}, args)
		return "real-install-project\n", nil
	}
	t.Cleanup(func() {
		runCommandOutputFn = savedOutput
	})

	assert.Equal(t, "real-install-project", composeProjectName(t.TempDir(), filepath.Join(t.TempDir(), ".env"), "new-api"))
}

func TestComposeProjectNameDefaultsToNewAPI(t *testing.T) {
	t.Setenv("UPDATER_COMPOSE_PROJECT_NAME", "")
	t.Setenv("COMPOSE_PROJECT_NAME", "")

	savedOutput := runCommandOutputFn
	runCommandOutputFn = func(_ string, _ string, _ ...string) (string, error) {
		return "", errors.New("container not found")
	}
	t.Cleanup(func() {
		runCommandOutputFn = savedOutput
	})

	assert.Equal(t, "new-api", composeProjectName(t.TempDir(), filepath.Join(t.TempDir(), ".env"), "new-api"))
}

func TestRunCommandIncludesCommandOutputInError(t *testing.T) {
	_, err := runCommandOutput("", "sh", "-c", "echo compose conflict >&2; exit 1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "compose conflict")
}

func TestPullPreparedImagePullsConfiguredTag(t *testing.T) {
	t.Setenv("UPDATER_IMAGE", "ghcr.io/artemk1337/new-api-v2")

	var gotDir string
	var gotName string
	var gotArgs []string
	saved := runCommandFn
	runCommandFn = func(dir string, name string, args ...string) error {
		gotDir = dir
		gotName = name
		gotArgs = append([]string(nil), args...)
		return nil
	}
	t.Cleanup(func() {
		runCommandFn = saved
	})

	got, err := pullPreparedImage("v1.2.3")
	require.NoError(t, err)
	assert.Equal(t, "ghcr.io/artemk1337/new-api-v2:v1.2.3", got)
	assert.Empty(t, gotDir)
	assert.Equal(t, "docker", gotName)
	assert.Equal(t, []string{"pull", "ghcr.io/artemk1337/new-api-v2:v1.2.3"}, gotArgs)
}

func TestExtractStatusVersion(t *testing.T) {
	assert.Equal(t, "v1.2.3", extractStatusVersion(`{"success":true,"version":"v1.2.3"}`))
	assert.Equal(t, "v1.2.4", extractStatusVersion(`{"success":true,"data":{"version":"v1.2.4"}}`))
	assert.Empty(t, extractStatusVersion(`{"success":true}`))
}

func TestHandleJobStatusWritesSnapshot(t *testing.T) {
	savedJobs := jobs
	jobs = map[string]*updateJob{}
	t.Cleanup(func() {
		jobs = savedJobs
	})

	jobs["update_1"] = &updateJob{
		JobID:   "update_1",
		Status:  "running",
		Step:    "pulling",
		Message: "pulling update image",
	}

	req := httptest.NewRequest(http.MethodGet, "/jobs/update_1", nil)
	recorder := httptest.NewRecorder()

	handleJobStatus(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"job_id":"update_1","status":"running","step":"pulling","message":"pulling update image"}`, recorder.Body.String())
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
