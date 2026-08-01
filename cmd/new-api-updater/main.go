package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type updateRequest struct {
	Tag string `json:"tag"`
}

type updateResponse struct {
	Accepted bool   `json:"accepted"`
	JobID    string `json:"job_id,omitempty"`
	Image    string `json:"image,omitempty"`
	Message  string `json:"message"`
}

type versionReadinessRequest struct {
	Tags []string `json:"tags"`
}

type versionReadiness struct {
	Tag           string `json:"tag"`
	Status        string `json:"status"`
	ReadyToDeploy bool   `json:"ready_to_deploy"`
	Error         string `json:"error,omitempty"`
}

type versionReadinessResponse struct {
	Versions []versionReadiness `json:"versions"`
}

type updateJob struct {
	JobID   string `json:"job_id"`
	Status  string `json:"status"`
	Step    string `json:"step"`
	Image   string `json:"image,omitempty"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

type serviceStatusResponse struct {
	Version string `json:"version"`
	Data    struct {
		Version string `json:"version"`
	} `json:"data"`
}

type telemetryAgentResponse struct {
	Running bool   `json:"running"`
	Message string `json:"message"`
}

type deployEnvSnapshot struct {
	Content         []byte
	Exists          bool
	PreviousVersion string
}

var (
	tagPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	updateMu     sync.Mutex
	updating     bool
	jobs         = map[string]*updateJob{}
	runCommandFn = runCommand
)

var (
	deployHealthTimeout  = 90 * time.Second
	deployHealthInterval = 3 * time.Second
	deployHandoffDelay   = 10 * time.Second
	jobRetention         = 24 * time.Hour
)

func main() {
	addr := env("UPDATER_ADDR", ":18090")
	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	http.HandleFunc("/update", handleUpdate)
	http.HandleFunc("/jobs/", handleJobStatus)
	http.HandleFunc("/versions/readiness", handleVersionReadiness)
	http.HandleFunc("/telemetry-agent", handleTelemetryAgent)
	log.Printf("new-api updater listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}

func handleTelemetryAgent(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		running, err := telemetryAgentRunning()
		if err != nil {
			writeAnyJSON(w, http.StatusInternalServerError, telemetryAgentResponse{Message: err.Error()})
			return
		}
		writeAnyJSON(w, http.StatusOK, telemetryAgentResponse{Running: running})
	case http.MethodPost:
		running, err := telemetryAgentRunning()
		if err != nil {
			writeAnyJSON(w, http.StatusInternalServerError, telemetryAgentResponse{Message: err.Error()})
			return
		}
		if running {
			writeAnyJSON(w, http.StatusOK, telemetryAgentResponse{Running: true, Message: "telemetry agent is already running"})
			return
		}
		if err := startTelemetryAgent(); err != nil {
			writeAnyJSON(w, http.StatusInternalServerError, telemetryAgentResponse{Message: err.Error()})
			return
		}
		writeAnyJSON(w, http.StatusOK, telemetryAgentResponse{Running: true, Message: "telemetry agent started"})
	case http.MethodDelete:
		running, err := telemetryAgentRunning()
		if err != nil {
			writeAnyJSON(w, http.StatusInternalServerError, telemetryAgentResponse{Message: err.Error()})
			return
		}
		if !running {
			writeAnyJSON(w, http.StatusOK, telemetryAgentResponse{Running: false, Message: "telemetry agent is already stopped"})
			return
		}
		if err := runCommandFn("", "docker", "stop", telemetryAgentContainer()); err != nil {
			writeAnyJSON(w, http.StatusInternalServerError, telemetryAgentResponse{Message: err.Error()})
			return
		}
		writeAnyJSON(w, http.StatusOK, telemetryAgentResponse{Running: false, Message: "telemetry agent stopped"})
	default:
		writeAnyJSON(w, http.StatusMethodNotAllowed, telemetryAgentResponse{Message: "method not allowed"})
	}
}

func telemetryAgentRunning() (bool, error) {
	output, err := runCommandOutputFn("", "docker", "inspect", "-f", "{{if .State.Running}}running{{else}}stopped{{end}}", telemetryAgentContainer())
	if err != nil {
		if strings.Contains(err.Error(), "No such object") {
			return false, nil
		}
		return false, err
	}
	return strings.TrimSpace(output) == "running", nil
}

func startTelemetryAgent() error {
	if err := runCommandFn("", "docker", "start", telemetryAgentContainer()); err == nil {
		return nil
	}
	composeDir := env("UPDATER_COMPOSE_DIR", "/workspace")
	envFile := env("UPDATER_ENV_FILE", filepath.Join(composeDir, ".env"))
	composeFile := env("UPDATER_COMPOSE_FILE", filepath.Join(composeDir, "docker-compose.yml"))
	projectName := composeProjectName(composeDir, envFile, env("UPDATER_SERVICE", "new-api"))
	return runCommandFn(composeDir, "docker", composeArgs(projectName, envFile, composeFile, "up", "-d", "--no-deps", telemetryAgentService())...)
}

func telemetryAgentContainer() string {
	return env("UPDATER_TELEMETRY_AGENT_CONTAINER", "new-api-system-telemetry-agent")
}

func telemetryAgentService() string {
	return env("UPDATER_TELEMETRY_AGENT_SERVICE", "system-telemetry-agent")
}

func handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, updateResponse{Message: "method not allowed"})
		return
	}
	var request updateRequest
	if err := common.DecodeJson(r.Body, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, updateResponse{Message: "invalid request body"})
		return
	}
	if err := validateRequest(request); err != nil {
		writeJSON(w, http.StatusBadRequest, updateResponse{Message: err.Error()})
		return
	}

	updateMu.Lock()
	if updating {
		updateMu.Unlock()
		writeJSON(w, http.StatusConflict, updateResponse{Message: "update is already running"})
		return
	}
	updating = true
	job := &updateJob{
		JobID:  fmt.Sprintf("update_%d", time.Now().UnixNano()),
		Status: "queued",
		Step:   "queued",
	}
	jobs[job.JobID] = job
	updateMu.Unlock()

	go func() {
		defer func() {
			updateMu.Lock()
			updating = false
			updateMu.Unlock()
		}()
		runUpdateJob(job.JobID, request.Tag)
	}()

	writeJSON(w, http.StatusAccepted, updateResponse{Accepted: true, JobID: job.JobID, Message: "update accepted"})
}

func handleJobStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, updateResponse{Message: "method not allowed"})
		return
	}
	jobID := strings.TrimPrefix(r.URL.Path, "/jobs/")
	updateMu.Lock()
	job := jobs[jobID]
	if job == nil {
		updateMu.Unlock()
		writeJSON(w, http.StatusNotFound, updateResponse{Message: "job not found"})
		return
	}
	jobCopy := *job
	updateMu.Unlock()
	writeAnyJSON(w, http.StatusOK, jobCopy)
}

func handleVersionReadiness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAnyJSON(w, http.StatusMethodNotAllowed, updateResponse{Message: "method not allowed"})
		return
	}
	request := versionReadinessRequest{}
	if err := common.DecodeJson(r.Body, &request); err != nil {
		writeAnyJSON(w, http.StatusBadRequest, updateResponse{Message: "invalid request body"})
		return
	}
	if len(request.Tags) == 0 {
		writeAnyJSON(w, http.StatusBadRequest, updateResponse{Message: "at least one tag is required"})
		return
	}

	versions := make([]versionReadiness, 0, len(request.Tags))
	seen := make(map[string]struct{}, len(request.Tags))
	for _, tag := range request.Tags {
		tag = strings.TrimSpace(tag)
		if !tagPattern.MatchString(tag) {
			writeAnyJSON(w, http.StatusBadRequest, updateResponse{Message: "invalid tag"})
			return
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}

		readiness := versionReadiness{Tag: tag, Status: "ready", ReadyToDeploy: true}
		if err := inspectPreparedImage(tag); err != nil {
			readiness.ReadyToDeploy = false
			readiness.Status = "unavailable"
			readiness.Error = imageReadinessError(err)
		}
		versions = append(versions, readiness)
	}
	writeAnyJSON(w, http.StatusOK, versionReadinessResponse{Versions: versions})
}

func runUpdateJob(jobID string, tag string) {
	setJobStatus(jobID, "running", "pulling", "", "", "pulling update image")
	imageTag, err := pullPreparedImage(tag)
	if err != nil {
		setJobStatus(jobID, "failed", "failed", "", err.Error(), "update image pull failed")
		return
	}

	setJobStatus(jobID, "deploying", "deploying", imageTag, "", "update image pulled; deploying service")
	time.Sleep(deployHandoffDelay)
	if err := deployPreparedImage(tag); err != nil {
		log.Printf("update deploy failed: %v", err)
		setJobStatus(jobID, "failed", "failed", imageTag, err.Error(), "update deploy failed")
		return
	}
	setJobStatus(jobID, "succeeded", "succeeded", imageTag, "", "update deployed")
}

func setJobStatus(jobID string, status string, step string, image string, errText string, message string) {
	updateMu.Lock()
	defer updateMu.Unlock()
	job := jobs[jobID]
	if job == nil {
		return
	}
	job.Status = status
	job.Step = step
	job.Image = image
	job.Error = errText
	job.Message = message
	if status == "succeeded" || status == "failed" {
		cleanupOldJobs(time.Now())
	}
}

func cleanupOldJobs(now time.Time) {
	cutoff := now.Add(-jobRetention).UnixNano()
	for jobID, job := range jobs {
		if job.Status != "succeeded" && job.Status != "failed" {
			continue
		}
		timestampText := strings.TrimPrefix(jobID, "update_")
		timestamp, err := strconv.ParseInt(timestampText, 10, 64)
		if err != nil || timestamp < cutoff {
			delete(jobs, jobID)
		}
	}
}

func validateRequest(request updateRequest) error {
	if !tagPattern.MatchString(request.Tag) {
		return errors.New("invalid tag")
	}
	return nil
}

func pullPreparedImage(tag string) (string, error) {
	imageTag := env("UPDATER_IMAGE", "ghcr.io/artemk1337/new-api-v2") + ":" + tag
	if err := runCommandFn("", "docker", "pull", imageTag); err != nil {
		return "", err
	}
	return imageTag, nil
}

func inspectPreparedImage(tag string) error {
	imageTag := env("UPDATER_IMAGE", "ghcr.io/artemk1337/new-api-v2") + ":" + tag
	return runCommandFn("", "docker", "manifest", "inspect", imageTag)
}

func imageReadinessError(err error) string {
	if err == nil {
		return ""
	}
	errorText := strings.ToLower(err.Error())
	if strings.Contains(errorText, "manifest unknown") ||
		strings.Contains(errorText, "no such manifest") ||
		strings.Contains(errorText, "name unknown") {
		return "image manifest is not published yet"
	}
	return err.Error()
}

func deployPreparedImage(tag string) error {
	composeDir := env("UPDATER_COMPOSE_DIR", "/workspace")
	envFile := env("UPDATER_ENV_FILE", filepath.Join(composeDir, ".env"))
	composeFile := env("UPDATER_COMPOSE_FILE", filepath.Join(composeDir, "docker-compose.yml"))
	service := env("UPDATER_SERVICE", "new-api")
	image := env("UPDATER_IMAGE", "ghcr.io/artemk1337/new-api-v2")
	projectName := composeProjectName(composeDir, envFile, service)
	previousEnv, err := readDeployEnvSnapshot(envFile)
	if err != nil {
		return err
	}
	if previousVersion, versionErr := serviceAPIVersion(composeDir, service); versionErr == nil {
		previousEnv = previousEnv.withPreviousVersion(previousVersion)
	}

	envUpdates := map[string]string{
		"NEW_API_IMAGE":   image,
		"NEW_API_VERSION": tag,
	}
	if err := upsertEnvFile(envFile, envUpdates); err != nil {
		return err
	}

	if err := runCommandFn(composeDir, "docker", composeArgs(projectName, envFile, composeFile, "up", "-d", "--no-deps", service)...); err != nil {
		return rollbackPreparedDeploy(composeDir, projectName, envFile, composeFile, service, previousEnv, err)
	}
	if err := waitServiceReady(composeDir, service, tag); err != nil {
		return rollbackPreparedDeploy(composeDir, projectName, envFile, composeFile, service, previousEnv, err)
	}
	return nil
}

func rollbackPreparedDeploy(composeDir string, projectName string, envFile string, composeFile string, service string, previousEnv deployEnvSnapshot, deployErr error) error {
	if rollbackErr := restoreDeployEnvSnapshot(envFile, previousEnv); rollbackErr != nil {
		return fmt.Errorf("%w; rollback env failed: %v", deployErr, rollbackErr)
	}
	if rollbackErr := runCommandFn(composeDir, "docker", composeArgs(projectName, envFile, composeFile, "up", "-d", "--no-deps", service)...); rollbackErr != nil {
		return fmt.Errorf("%w; rollback deploy failed: %v", deployErr, rollbackErr)
	}
	if rollbackErr := waitServiceReady(composeDir, service, previousEnv.PreviousVersion); rollbackErr != nil {
		return fmt.Errorf("%w; rollback health check failed: %v", deployErr, rollbackErr)
	}
	return deployErr
}

func composeArgs(projectName string, envFile string, composeFile string, command ...string) []string {
	args := []string{"compose", "-p", projectName, "--env-file", envFile, "-f", composeFile}
	return append(args, command...)
}

func composeProjectName(composeDir string, envFile string, service string) string {
	if projectName := env("UPDATER_COMPOSE_PROJECT_NAME", ""); projectName != "" {
		return projectName
	}
	if projectName := env("COMPOSE_PROJECT_NAME", ""); projectName != "" {
		return projectName
	}
	if projectName := readEnvValue(envFile, "COMPOSE_PROJECT_NAME"); projectName != "" {
		return projectName
	}
	if projectName := existingComposeProjectName(composeDir, service); projectName != "" {
		return projectName
	}
	return "new-api"
}

func existingComposeProjectName(composeDir string, service string) string {
	output, err := runCommandOutputFn(composeDir, "docker", "inspect", "-f", `{{ index .Config.Labels "com.docker.compose.project" }}`, service)
	if err != nil {
		return ""
	}
	projectName := strings.TrimSpace(output)
	if projectName == "" || projectName == "<no value>" {
		return ""
	}
	return projectName
}

func waitServiceReady(composeDir string, service string, expectedVersion string) error {
	deadline := time.Now().Add(deployHealthTimeout)
	for {
		status, err := serviceHealthStatus(composeDir, service)
		if err == nil && (status == "healthy" || status == "running") {
			if expectedVersion == "" {
				return nil
			}
			if version, versionErr := serviceAPIVersion(composeDir, service); versionErr == nil && version == expectedVersion {
				return nil
			}
		}
		if time.Now().After(deadline) {
			if err != nil {
				return err
			}
			return fmt.Errorf("service %s did not become ready", service)
		}
		time.Sleep(deployHealthInterval)
	}
}

func serviceHealthStatus(composeDir string, service string) (string, error) {
	output, err := runCommandOutputFn(composeDir, "docker", "inspect", "-f", "{{if .State.Health}}{{.State.Health.Status}}{{else if .State.Running}}running{{else}}stopped{{end}}", service)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func serviceAPIVersion(composeDir string, service string) (string, error) {
	output, err := runCommandOutputFn(composeDir, "docker", "exec", service, "wget", "-q", "-O", "-", "http://localhost:3000/api/status")
	if err != nil {
		return "", err
	}
	version := extractStatusVersion(output)
	if version == "" {
		return "", errors.New("service status response has no version")
	}
	return version, nil
}

func extractStatusVersion(response string) string {
	status := serviceStatusResponse{}
	if err := common.UnmarshalJsonStr(response, &status); err != nil {
		return ""
	}
	if status.Data.Version != "" {
		return status.Data.Version
	}
	return status.Version
}

func readDeployEnvSnapshot(path string) (deployEnvSnapshot, error) {
	content, err := os.ReadFile(path)
	if err == nil {
		return deployEnvSnapshot{Content: content, Exists: true}, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return deployEnvSnapshot{}, nil
	}
	return deployEnvSnapshot{}, err
}

func (snapshot deployEnvSnapshot) withPreviousVersion(version string) deployEnvSnapshot {
	snapshot.PreviousVersion = strings.TrimSpace(version)
	return snapshot
}

func restoreDeployEnvSnapshot(path string, snapshot deployEnvSnapshot) error {
	if !snapshot.Exists {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, snapshot.Content, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func upsertEnvFile(path string, updates map[string]string) error {
	lines := make([]string, 0)
	seen := map[string]bool{}

	file, err := os.Open(path)
	if err == nil {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			key, value, ok := strings.Cut(line, "=")
			_ = value
			trimmedKey := strings.TrimSpace(key)
			if !ok || trimmedKey == "" || strings.HasPrefix(trimmedKey, "#") {
				lines = append(lines, line)
				continue
			}
			if update, exists := updates[trimmedKey]; exists {
				lines = append(lines, trimmedKey+"="+update)
				seen[trimmedKey] = true
				continue
			}
			lines = append(lines, line)
		}
		if scanErr := scanner.Err(); scanErr != nil {
			_ = file.Close()
			return scanErr
		}
		_ = file.Close()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	keys := make([]string, 0, len(updates))
	for key := range updates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !seen[key] {
			lines = append(lines, key+"="+updates[key])
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	var builder strings.Builder
	for _, line := range lines {
		builder.WriteString(line)
		builder.WriteString("\n")
	}
	if err := os.WriteFile(tmp, []byte(builder.String()), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readEnvValue(path string, targetKey string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if !ok || strings.TrimSpace(key) != targetKey {
			continue
		}
		value = strings.TrimSpace(value)
		return strings.Trim(value, `"'`)
	}
	return ""
}

func runCommand(dir string, name string, args ...string) error {
	_, err := runCommandOutput(dir, name, args...)
	return err
}

var runCommandOutputFn = runCommandOutput

func runCommandOutput(dir string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	text := string(bytes.TrimSpace(output))
	if text != "" {
		log.Print(text)
	}
	if err != nil {
		if text != "" {
			return text, fmt.Errorf("%s %s failed: %w: %s", name, strings.Join(args, " "), err, text)
		}
		return text, fmt.Errorf("%s %s failed: %w", name, strings.Join(args, " "), err)
	}
	return text, nil
}

func writeJSON(w http.ResponseWriter, status int, response updateResponse) {
	writeAnyJSON(w, status, response)
}

func writeAnyJSON(w http.ResponseWriter, status int, response any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	data, err := common.Marshal(response)
	if err != nil {
		log.Printf("write response failed: %v", err)
		return
	}
	if _, err := w.Write(data); err != nil {
		log.Printf("write response failed: %v", err)
	}
}

func env(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
