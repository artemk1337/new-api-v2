package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleUpdateRejectsInvalidTag(t *testing.T) {
	recorder := httptest.NewRecorder()
	handleUpdate(recorder, httptest.NewRequest(http.MethodPost, "/update", strings.NewReader(`{"tag":"v1.0.1;rm"}`)))
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.JSONEq(t, `{"accepted":false,"message":"invalid tag"}`, recorder.Body.String())
}

func TestHandleVersionReadinessMarksManifestUnavailable(t *testing.T) {
	saved := runCommandFn
	runCommandFn = func(_ string, _ string, _ ...string) error { return fmt.Errorf("manifest unknown") }
	t.Cleanup(func() { runCommandFn = saved })
	recorder := httptest.NewRecorder()
	handleVersionReadiness(recorder, httptest.NewRequest(http.MethodPost, "/versions/readiness", strings.NewReader(`{"tags":["v1.0.1","v1.0.1"]}`)))
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"versions":[{"tag":"v1.0.1","status":"unavailable","ready_to_deploy":false,"error":"image manifest is not published yet"}]}`, recorder.Body.String())
}

func TestTelemetryAgentStatusTreatsMissingContainerAsStopped(t *testing.T) {
	saved := runCommandOutputFn
	runCommandOutputFn = func(string, string, ...string) (string, error) { return "", assert.AnError }
	t.Cleanup(func() { runCommandOutputFn = saved })
	running, err := telemetryAgentRunning()
	require.Error(t, err)
	assert.False(t, running)
}

func TestHandleTelemetryAgentStartIsIdempotentWhenRunning(t *testing.T) {
	savedOutput, savedCommand := runCommandOutputFn, runCommandFn
	t.Cleanup(func() {
		runCommandOutputFn, runCommandFn = savedOutput, savedCommand
	})
	runCommandOutputFn = func(_ string, name string, args ...string) (string, error) {
		require.Equal(t, "docker", name)
		require.Equal(t, []string{"inspect", "-f", "{{if .State.Running}}running{{else}}stopped{{end}}", telemetryAgentContainer()}, args)
		return "running", nil
	}
	runCommandFn = func(_ string, name string, args ...string) error {
		t.Fatalf("unexpected %s %v", name, args)
		return nil
	}

	recorder := httptest.NewRecorder()
	handleTelemetryAgent(recorder, httptest.NewRequest(http.MethodPost, "/telemetry-agent", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"running":true,"message":""}`, recorder.Body.String())
}

func TestHandleTelemetryAgentStopIsIdempotentWhenStopped(t *testing.T) {
	savedOutput, savedCommand := runCommandOutputFn, runCommandFn
	t.Cleanup(func() {
		runCommandOutputFn, runCommandFn = savedOutput, savedCommand
	})
	runCommandOutputFn = func(_ string, name string, args ...string) (string, error) {
		require.Equal(t, "docker", name)
		return "stopped", nil
	}
	runCommandFn = func(_ string, name string, args ...string) error {
		t.Fatalf("unexpected %s %v", name, args)
		return nil
	}

	recorder := httptest.NewRecorder()
	handleTelemetryAgent(recorder, httptest.NewRequest(http.MethodDelete, "/telemetry-agent", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"running":false,"message":""}`, recorder.Body.String())
}

func TestHandleTelemetryAgentStartReportsMissingContainer(t *testing.T) {
	savedOutput, savedCommand := runCommandOutputFn, runCommandFn
	t.Cleanup(func() {
		runCommandOutputFn, runCommandFn = savedOutput, savedCommand
	})
	runCommandOutputFn = func(_ string, _ string, _ ...string) (string, error) {
		return "", fmt.Errorf("docker inspect failed: No such object")
	}
	runCommandFn = func(_ string, _ string, _ ...string) error {
		return fmt.Errorf("docker start failed: No such container")
	}

	recorder := httptest.NewRecorder()
	handleTelemetryAgent(recorder, httptest.NewRequest(http.MethodPost, "/telemetry-agent", nil))
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "does not exist; create it on the Docker host")
}
