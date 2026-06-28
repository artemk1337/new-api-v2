package service

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func withSystemUpdateHTTPClient(t *testing.T, fn roundTripFunc) {
	t.Helper()
	saved := systemUpdateHTTPClient
	systemUpdateHTTPClient = &http.Client{Transport: fn}
	t.Cleanup(func() {
		systemUpdateHTTPClient = saved
	})
}

func TestCheckSystemUpdateUsesConfiguredRepository(t *testing.T) {
	t.Setenv("UPDATE_CHECK_REPOSITORY", "artemk1337/new-api-v2")
	savedVersion := common.Version
	common.Version = "v1.0.0"
	t.Cleanup(func() {
		common.Version = savedVersion
	})

	withSystemUpdateHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host {
		case "api.github.com":
			assert.Equal(t, "/repos/artemk1337/new-api-v2/git/matching-refs/tags/", req.URL.Path)
			return jsonResponse(http.StatusOK, `[
				{"ref":"refs/tags/v1.0.1-rc.1"},
				{"ref":"refs/tags/v1.0.1"},
				{"ref":"refs/tags/v1.1.0"}
			]`), nil
		case "raw.githubusercontent.com":
			assert.Equal(t, "/artemk1337/new-api-v2/v1.1.0/CHANGELOG.md", req.URL.Path)
			return jsonResponse(http.StatusOK, "# Changelog\n\n## v1.1.0\n\n- New release\n\n## v1.0.1\n\n- Patch release\n"), nil
		default:
			t.Fatalf("unexpected request host: %s", req.URL.Host)
		}
		return nil, nil
	})

	result, err := CheckSystemUpdate(t.Context())
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Enabled)
	assert.False(t, result.CanUpdate)
	assert.Equal(t, "artemk1337/new-api-v2", result.Repository)
	assert.Equal(t, "v1.0.0", result.CurrentVersion)
	assert.Equal(t, "v1.1.0", result.LatestVersion)
	assert.True(t, result.UpdateAvailable)
	require.NotNil(t, result.Release)
	assert.Equal(t, "v1.1.0", result.Release.TagName)
	assert.Equal(t, "https://github.com/artemk1337/new-api-v2/tree/v1.1.0", result.Release.HTMLURL)
	require.Len(t, result.Releases, 2)
	assert.Equal(t, "v1.0.1", result.Releases[0].TagName)
	assert.Contains(t, result.Releases[0].Body, "Patch release")
	assert.Equal(t, "v1.1.0", result.Releases[1].TagName)
	assert.Contains(t, result.Releases[1].Body, "New release")
}

func TestCheckSystemUpdateIgnoresPrereleaseTags(t *testing.T) {
	t.Setenv("UPDATE_CHECK_REPOSITORY", "artemk1337/new-api-v2")
	savedVersion := common.Version
	common.Version = "v1.0.0"
	t.Cleanup(func() {
		common.Version = savedVersion
	})

	withSystemUpdateHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host {
		case "api.github.com":
			assert.Equal(t, "/repos/artemk1337/new-api-v2/git/matching-refs/tags/", req.URL.Path)
			return jsonResponse(http.StatusOK, `[
				{"ref":"refs/tags/v2.0.0-rc.1"},
				{"ref":"refs/tags/v1.9.0"}
			]`), nil
		case "raw.githubusercontent.com":
			return jsonResponse(http.StatusOK, "# Changelog\n\n## v1.9.0\n\n- Stable\n"), nil
		default:
			t.Fatalf("unexpected request host: %s", req.URL.Host)
		}
		return nil, nil
	})

	result, err := CheckSystemUpdate(t.Context())
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "v1.9.0", result.LatestVersion)
}

func TestStartSystemUpdateTaskDedupsActiveRun(t *testing.T) {
	truncate(t)

	first, created, err := StartSystemUpdateTask("v1.0.1")
	require.NoError(t, err)
	require.True(t, created)
	require.NotNil(t, first)

	second, created, err := StartSystemUpdateTask("v1.0.2")
	require.NoError(t, err)
	require.False(t, created)
	require.NotNil(t, second)
	assert.Equal(t, first.TaskID, second.TaskID)
}

func TestRunSystemUpdateTaskValidatesTagAndRequestsUpdater(t *testing.T) {
	truncate(t)
	t.Setenv("UPDATE_CHECK_REPOSITORY", "artemk1337/new-api-v2")
	t.Setenv("UPDATE_SIDECAR_TOKEN", "secret")
	savedVersion := common.Version
	common.Version = "v1.0.0"
	t.Cleanup(func() {
		common.Version = savedVersion
	})

	var updaterCalled bool
	var statusCalled bool
	withSystemUpdateHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host {
		case "api.github.com":
			assert.Equal(t, "/repos/artemk1337/new-api-v2/git/ref/tags/v1.0.1", req.URL.Path)
			return jsonResponse(http.StatusOK, `{"ref":"refs/tags/v1.0.1"}`), nil
		case "new-api-updater:18090":
			assert.Equal(t, "Bearer secret", req.Header.Get("Authorization"))
			switch req.Method {
			case http.MethodPost:
				updaterCalled = true
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				assert.JSONEq(t, `{"tag":"v1.0.1"}`, string(body))
				return jsonResponse(http.StatusAccepted, `{"accepted":true,"job_id":"job-1","message":"update accepted"}`), nil
			case http.MethodGet:
				statusCalled = true
				assert.Equal(t, "/jobs/job-1", req.URL.Path)
				return jsonResponse(http.StatusOK, `{"job_id":"job-1","status":"deploying","step":"deploying","image":"ghcr.io/artemk1337/new-api-v2:v1.0.1","message":"update image pulled; deploying service"}`), nil
			}
		default:
			t.Fatalf("unexpected request host: %s", req.URL.Host)
		}
		return nil, nil
	})

	task, err := model.CreateSystemTask(model.SystemTaskTypeSystemUpdate, SystemUpdatePayload{Version: "v1.0.1"}, nil)
	require.NoError(t, err)
	claimed, ok, err := model.ClaimSystemTask(task.ID, model.SystemTaskTypeSystemUpdate, "runner-a", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, RunSystemUpdateTask(t.Context(), claimed, "runner-a"))
	assert.True(t, updaterCalled)
	assert.True(t, statusCalled)

	reloaded, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	require.NotNil(t, reloaded)
	assert.Equal(t, model.SystemTaskStatusSucceeded, reloaded.Status)
	assert.Contains(t, reloaded.Result, `"requested_version":"v1.0.1"`)
	assert.Contains(t, reloaded.Result, `"image":"ghcr.io/artemk1337/new-api-v2:v1.0.1"`)
	assert.Contains(t, reloaded.Result, `"job_id":"job-1"`)
	assert.Contains(t, reloaded.Result, `"status":"deploying"`)
}

func TestRunSystemUpdateTaskRequiresSidecarToken(t *testing.T) {
	truncate(t)
	t.Setenv("UPDATE_CHECK_REPOSITORY", "artemk1337/new-api-v2")
	savedVersion := common.Version
	common.Version = "v1.0.0"
	t.Cleanup(func() {
		common.Version = savedVersion
	})

	withSystemUpdateHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "api.github.com", req.URL.Host)
		return jsonResponse(http.StatusOK, `{"ref":"refs/tags/v1.0.1"}`), nil
	})

	task, err := model.CreateSystemTask(model.SystemTaskTypeSystemUpdate, SystemUpdatePayload{Version: "v1.0.1"}, nil)
	require.NoError(t, err)
	claimed, ok, err := model.ClaimSystemTask(task.ID, model.SystemTaskTypeSystemUpdate, "runner-a", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, ok)

	err = RunSystemUpdateTask(t.Context(), claimed, "runner-a")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update sidecar token is required")
}
