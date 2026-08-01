package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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

func TestScheduleUpdaterSelfUpgradeStartsDetachedHelper(t *testing.T) {
	t.Setenv("UPDATER_HOST_COMPOSE_DIR", "/root/repos/new-api")
	t.Setenv("UPDATER_COMPOSE_DIR", "/workspace")
	t.Setenv("UPDATER_ENV_FILE", "/workspace/.env")
	t.Setenv("UPDATER_COMPOSE_FILE", "/workspace/docker-compose.yml")
	t.Setenv("UPDATER_COMPOSE_PROJECT_NAME", "production-api")
	t.Setenv("UPDATER_SIDECAR_IMAGE", "ghcr.io/artemk1337/new-api-v2-updater")

	saved := runCommandFn
	runCommandFn = func(dir string, name string, args ...string) error {
		assert.Empty(t, dir)
		require.Equal(t, "docker", name)
		assert.Equal(t, "run", args[0])
		assert.Contains(t, args, "-d")
		assert.Contains(t, args, "--rm")
		assert.Contains(t, args, "/var/run/docker.sock:/var/run/docker.sock")
		assert.Contains(t, args, "/root/repos/new-api:/workspace")
		assert.Contains(t, args, "ghcr.io/artemk1337/new-api-v2-updater:v1.2.3")
		assert.Contains(t, args[len(args)-1], "UPDATER_SIDECAR_VERSION=v1.2.3")
		assert.Contains(t, args[len(args)-1], "up -d --no-deps new-api-updater")
		return nil
	}
	t.Cleanup(func() { runCommandFn = saved })

	require.NoError(t, scheduleUpdaterSelfUpgrade("v1.2.3"))
}

func TestUpdaterSelfUpgradeScriptIsValidShell(t *testing.T) {
	script := updaterSelfUpgradeScript("production-api", ".env", "docker-compose.yml", "new-api-updater", "v1.2.3")
	command := exec.Command("sh", "-n")
	command.Stdin = strings.NewReader(script)
	require.NoError(t, command.Run())
}

func TestUpdaterHostComposeDirFailsWithoutWorkspaceMount(t *testing.T) {
	t.Setenv("UPDATER_HOST_COMPOSE_DIR", "")
	saved := runCommandOutputFn
	runCommandOutputFn = func(string, string, ...string) (string, error) {
		return "", nil
	}
	t.Cleanup(func() { runCommandOutputFn = saved })

	_, err := updaterHostComposeDir()
	require.EqualError(t, err, "updater workspace bind mount is not available")
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

func TestHandleVersionReadinessReturnsBatchStatuses(t *testing.T) {
	t.Setenv("UPDATER_IMAGE", "ghcr.io/artemk1337/new-api-v2")
	saved := runCommandFn
	runCommandFn = func(_ string, name string, args ...string) error {
		require.Equal(t, "docker", name)
		require.Equal(t, []string{"manifest", "inspect", "ghcr.io/artemk1337/new-api-v2:v1.0.1"}, args)
		return nil
	}
	t.Cleanup(func() {
		runCommandFn = saved
	})

	recorder := httptest.NewRecorder()
	handleVersionReadiness(recorder, httptest.NewRequest(http.MethodPost, "/versions/readiness", strings.NewReader(`{"tags":["v1.0.1","v1.0.1"]}`)))

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"versions":[{"tag":"v1.0.1","status":"ready","ready_to_deploy":true}]}`, recorder.Body.String())
}

func TestHandleVersionReadinessMarksUnconfirmedImagesUnavailable(t *testing.T) {
	saved := runCommandFn
	runCommandFn = func(_ string, _ string, args ...string) error {
		switch args[2] {
		case "ghcr.io/artemk1337/new-api-v2:v1.0.1":
			return errors.New("manifest unknown")
		case "ghcr.io/artemk1337/new-api-v2:v1.0.2":
			return errors.New("dial tcp: lookup ghcr.io: no such host")
		default:
			t.Fatalf("unexpected image: %s", args[2])
		}
		return nil
	}
	t.Cleanup(func() {
		runCommandFn = saved
	})

	recorder := httptest.NewRecorder()
	handleVersionReadiness(recorder, httptest.NewRequest(http.MethodPost, "/versions/readiness", strings.NewReader(`{"tags":["v1.0.1","v1.0.2"]}`)))

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"versions":[
		{"tag":"v1.0.1","status":"unavailable","ready_to_deploy":false,"error":"image manifest is not published yet"},
		{"tag":"v1.0.2","status":"unavailable","ready_to_deploy":false,"error":"dial tcp: lookup ghcr.io: no such host"}
	]}`, recorder.Body.String())
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

func TestTelemetryAgentStatusTreatsMissingContainerAsStopped(t *testing.T) {
	previousOutput := runCommandOutputFn
	runCommandOutputFn = func(string, string, ...string) (string, error) {
		return "", errors.New("No such object: new-api-system-telemetry-agent")
	}
	t.Cleanup(func() { runCommandOutputFn = previousOutput })

	recorder := httptest.NewRecorder()
	handleTelemetryAgent(recorder, httptest.NewRequest(http.MethodGet, "/telemetry-agent", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"running":false,"message":""}`, recorder.Body.String())
}

func TestTelemetryAgentStartIsNoopWhenAlreadyRunning(t *testing.T) {
	previousOutput := runCommandOutputFn
	previousRun := runCommandFn
	runCommandOutputFn = func(string, string, ...string) (string, error) {
		return "running", nil
	}
	called := false
	runCommandFn = func(string, string, ...string) error {
		called = true
		return nil
	}
	t.Cleanup(func() {
		runCommandOutputFn = previousOutput
		runCommandFn = previousRun
	})

	recorder := httptest.NewRecorder()
	handleTelemetryAgent(recorder, httptest.NewRequest(http.MethodPost, "/telemetry-agent", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"running":true,"message":"telemetry agent is already running"}`, recorder.Body.String())
	assert.False(t, called)
}

func TestTelemetryAgentStopUsesFixedContainer(t *testing.T) {
	previousOutput := runCommandOutputFn
	previousRun := runCommandFn
	runCommandOutputFn = func(string, string, ...string) (string, error) {
		return "running", nil
	}
	runCommandFn = func(_ string, name string, args ...string) error {
		require.Equal(t, "docker", name)
		require.Equal(t, []string{"stop", "new-api-system-telemetry-agent"}, args)
		return nil
	}
	t.Cleanup(func() {
		runCommandOutputFn = previousOutput
		runCommandFn = previousRun
	})

	recorder := httptest.NewRecorder()
	handleTelemetryAgent(recorder, httptest.NewRequest(http.MethodDelete, "/telemetry-agent", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"running":false,"message":"telemetry agent stopped"}`, recorder.Body.String())
}
