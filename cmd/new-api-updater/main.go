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

type deployEnvSnapshot struct {
	Content         []byte
	Exists          bool
	PreviousVersion string
}

type telemetryAgentResponse struct {
	Running bool   `json:"running"`
	Message string `json:"message"`
}

var (
	tagPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	updateMu     sync.Mutex
	updating     bool
	jobs         = map[string]*updateJob{}
	runCommandFn = runCommand
)

var (
	runCommandOutputFn   = runCommandOutput
	deployHealthTimeout  = 90 * time.Second
	deployHealthInterval = 3 * time.Second
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
			writeAnyJSON(w, http.StatusOK, telemetryAgentResponse{Running: true})
			return
		}
		if err := startTelemetryAgent(); err != nil {
			writeAnyJSON(w, http.StatusInternalServerError, telemetryAgentResponse{Message: err.Error()})
			return
		}
		writeAnyJSON(w, http.StatusOK, telemetryAgentResponse{Running: true})
	case http.MethodDelete:
		running, err := telemetryAgentRunning()
		if err != nil {
			writeAnyJSON(w, http.StatusInternalServerError, telemetryAgentResponse{Message: err.Error()})
			return
		}
		if !running {
			writeAnyJSON(w, http.StatusOK, telemetryAgentResponse{Running: false})
			return
		}
		if err := runCommandFn("", "docker", "stop", telemetryAgentContainer()); err != nil {
			writeAnyJSON(w, http.StatusInternalServerError, telemetryAgentResponse{Message: err.Error()})
			return
		}
		writeAnyJSON(w, http.StatusOK, telemetryAgentResponse{Running: false})
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
	if err := runCommandFn("", "docker", "start", telemetryAgentContainer()); err != nil {
		if isMissingTelemetryAgentError(err) {
			return fmt.Errorf("telemetry agent container %q does not exist; create it on the Docker host before enabling telemetry: %w", telemetryAgentContainer(), err)
		}
		return err
	}
	return nil
}

func isMissingTelemetryAgentError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such object") || strings.Contains(message, "no such container")
}

func telemetryAgentContainer() string {
	return env("UPDATER_TELEMETRY_AGENT_CONTAINER", "new-api-system-telemetry-agent")
}

func handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, updateResponse{Message: "method not allowed"})
		return
	}
	request := updateRequest{}
	if err := common.DecodeJson(r.Body, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, updateResponse{Message: "invalid request body"})
		return
	}
	if !tagPattern.MatchString(request.Tag) {
		writeJSON(w, http.StatusBadRequest, updateResponse{Message: "invalid tag"})
		return
	}

	updateMu.Lock()
	defer updateMu.Unlock()
	if updating {
		writeJSON(w, http.StatusConflict, updateResponse{Message: "update is already running"})
		return
	}
	updating = true
	job := &updateJob{JobID: fmt.Sprintf("update_%d", time.Now().UnixNano()), Status: "queued", Step: "queued"}
	jobs[job.JobID] = job
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
	updateMu.Lock()
	job := jobs[strings.TrimPrefix(r.URL.Path, "/jobs/")]
	if job != nil {
		copy := *job
		job = &copy
	}
	updateMu.Unlock()
	if job == nil {
		writeJSON(w, http.StatusNotFound, updateResponse{Message: "job not found"})
		return
	}
	writeAnyJSON(w, http.StatusOK, job)
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
		item := versionReadiness{Tag: tag, Status: "ready", ReadyToDeploy: true}
		if err := inspectPreparedImage(tag); err != nil {
			item.Status = "unavailable"
			item.ReadyToDeploy = false
			item.Error = imageReadinessError(err)
		}
		versions = append(versions, item)
	}
	writeAnyJSON(w, http.StatusOK, versionReadinessResponse{Versions: versions})
}

func runUpdateJob(jobID string, tag string) {
	if err := inspectPreparedImage(tag); err != nil {
		setJobStatus(jobID, "failed", "failed", "", err.Error(), "update image is not ready")
		return
	}
	setJobStatus(jobID, "running", "pulling", "", "", "pulling update image")
	image := env("UPDATER_IMAGE", "ghcr.io/artemk1337/new-api-v2") + ":" + tag
	if err := runCommandFn("", "docker", "pull", image); err != nil {
		setJobStatus(jobID, "failed", "failed", "", err.Error(), "update image pull failed")
		return
	}
	setJobStatus(jobID, "deploying", "deploying", image, "", "deploying service")
	if err := deployPreparedImage(tag); err != nil {
		setJobStatus(jobID, "failed", "failed", image, err.Error(), "update deploy failed")
		return
	}
	setJobStatus(jobID, "succeeded", "succeeded", image, "", "update deployed")
}

func setJobStatus(jobID, status, step, image, errText, message string) {
	updateMu.Lock()
	defer updateMu.Unlock()
	job := jobs[jobID]
	if job == nil {
		return
	}
	job.Status, job.Step, job.Image, job.Error, job.Message = status, step, image, errText, message
	if status == "succeeded" || status == "failed" {
		cutoff := time.Now().Add(-jobRetention).UnixNano()
		for id, old := range jobs {
			if (old.Status == "succeeded" || old.Status == "failed") && strings.TrimPrefix(id, "update_") != id {
				stamp, err := strconv.ParseInt(strings.TrimPrefix(id, "update_"), 10, 64)
				if err == nil && stamp < cutoff {
					delete(jobs, id)
				}
			}
		}
	}
}

func inspectPreparedImage(tag string) error {
	return runCommandFn("", "docker", "manifest", "inspect", env("UPDATER_IMAGE", "ghcr.io/artemk1337/new-api-v2")+":"+tag)
}

func imageReadinessError(err error) string {
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "manifest unknown") || strings.Contains(text, "no such manifest") || strings.Contains(text, "name unknown") {
		return "image manifest is not published yet"
	}
	return err.Error()
}

func deployPreparedImage(tag string) error {
	composeDir, err := updaterComposeDir()
	if err != nil {
		return err
	}
	envFile := env("UPDATER_ENV_FILE", filepath.Join(composeDir, ".env"))
	composeFile := env("UPDATER_COMPOSE_FILE", filepath.Join(composeDir, "docker-compose.yml"))
	service := env("UPDATER_SERVICE", "new-api")
	projectName := composeProjectName(envFile, service)
	if err := backupDatabase(composeDir, projectName, envFile, composeFile); err != nil {
		return err
	}
	previous, err := readDeployEnvSnapshot(envFile)
	if err != nil {
		return err
	}
	if version, versionErr := serviceAPIVersion(service); versionErr == nil {
		previous.PreviousVersion = version
	}
	if err := upsertEnvFile(envFile, map[string]string{"NEW_API_IMAGE": env("UPDATER_IMAGE", "ghcr.io/artemk1337/new-api-v2"), "NEW_API_VERSION": tag}); err != nil {
		return err
	}
	args := composeArgs(projectName, envFile, composeFile, "up", "-d", "--no-deps", service)
	if err := runCommandFn(composeDir, "docker", args...); err != nil {
		return rollbackPreparedDeploy(composeDir, projectName, envFile, composeFile, service, previous, err)
	}
	if err := waitServiceReady(service, tag); err != nil {
		return rollbackPreparedDeploy(composeDir, projectName, envFile, composeFile, service, previous, err)
	}
	return nil
}

func backupDatabase(composeDir, projectName, envFile, composeFile string) error {
	backupDir := filepath.Join(composeDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return err
	}
	backupPath := filepath.Join(backupDir, "new-api-before-update-"+time.Now().UTC().Format("20060102T150405Z")+".dump")
	backup, err := os.Create(backupPath)
	if err != nil {
		return err
	}
	args := composeArgs(projectName, envFile, composeFile, "exec", "-T", env("UPDATER_DATABASE_SERVICE", "postgres"), "sh", "-c", `pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc`)
	command := exec.Command("docker", args...)
	command.Dir = composeDir
	command.Stdout = backup
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		_ = backup.Close()
		return fmt.Errorf("create database backup: %w", err)
	}
	if err := backup.Close(); err != nil {
		return err
	}
	input, err := os.Open(backupPath)
	if err != nil {
		return err
	}
	defer input.Close()
	args = composeArgs(projectName, envFile, composeFile, "exec", "-T", env("UPDATER_DATABASE_SERVICE", "postgres"), "pg_restore", "-l")
	command = exec.Command("docker", args...)
	command.Dir = composeDir
	command.Stdin = input
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("verify database backup: %w", err)
	}
	return nil
}

func updaterComposeDir() (string, error) {
	composeDir := env("UPDATER_COMPOSE_DIR", "")
	if composeDir == "" {
		return "", errors.New("UPDATER_COMPOSE_DIR is not configured")
	}
	if _, err := os.Stat(composeDir); err != nil {
		return "", fmt.Errorf("updater compose directory is not mounted: %w", err)
	}
	return composeDir, nil
}

func rollbackPreparedDeploy(composeDir, projectName, envFile, composeFile, service string, previous deployEnvSnapshot, deployErr error) error {
	if err := restoreDeployEnvSnapshot(envFile, previous); err != nil {
		return fmt.Errorf("%w; rollback env failed: %v", deployErr, err)
	}
	if err := runCommandFn(composeDir, "docker", composeArgs(projectName, envFile, composeFile, "up", "-d", "--no-deps", service)...); err != nil {
		return fmt.Errorf("%w; rollback deploy failed: %v", deployErr, err)
	}
	if err := waitServiceReady(service, previous.PreviousVersion); err != nil {
		return fmt.Errorf("%w; rollback health check failed: %v", deployErr, err)
	}
	return deployErr
}

func composeArgs(projectName, envFile, composeFile string, command ...string) []string {
	args := []string{"compose", "-p", projectName, "--env-file", envFile, "-f", composeFile}
	return append(args, command...)
}

func composeProjectName(envFile, service string) string {
	if name := env("UPDATER_COMPOSE_PROJECT_NAME", ""); name != "" {
		return name
	}
	if name := readEnvValue(envFile, "COMPOSE_PROJECT_NAME"); name != "" {
		return name
	}
	output, err := runCommandOutputFn("", "docker", "inspect", "-f", `{{ index .Config.Labels "com.docker.compose.project" }}`, service)
	if err == nil && strings.TrimSpace(output) != "" && strings.TrimSpace(output) != "<no value>" {
		return strings.TrimSpace(output)
	}
	return "new-api"
}

func waitServiceReady(service, expectedVersion string) error {
	deadline := time.Now().Add(deployHealthTimeout)
	for {
		status, err := serviceHealthStatus(service)
		if err == nil && (status == "healthy" || status == "running") {
			if expectedVersion == "" {
				return nil
			}
			if version, err := serviceAPIVersion(service); err == nil && version == expectedVersion {
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

func serviceHealthStatus(service string) (string, error) {
	output, err := runCommandOutputFn("", "docker", "inspect", "-f", "{{if .State.Health}}{{.State.Health.Status}}{{else if .State.Running}}running{{else}}stopped{{end}}", service)
	return strings.TrimSpace(output), err
}

func serviceAPIVersion(service string) (string, error) {
	output, err := runCommandOutputFn("", "docker", "exec", service, "wget", "-q", "-O", "-", "http://localhost:3000/api/status")
	if err != nil {
		return "", err
	}
	status := serviceStatusResponse{}
	if err := common.UnmarshalJsonStr(output, &status); err != nil {
		return "", err
	}
	if status.Data.Version != "" {
		return status.Data.Version, nil
	}
	if status.Version == "" {
		return "", errors.New("service status response has no version")
	}
	return status.Version, nil
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

func restoreDeployEnvSnapshot(path string, snapshot deployEnvSnapshot) error {
	if !snapshot.Exists {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	return os.WriteFile(path, snapshot.Content, 0644)
}

func upsertEnvFile(path string, updates map[string]string) error {
	lines, seen := make([]string, 0), map[string]bool{}
	file, err := os.Open(path)
	if err == nil {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			key, _, ok := strings.Cut(line, "=")
			key = strings.TrimSpace(key)
			if !ok || key == "" || strings.HasPrefix(key, "#") {
				lines = append(lines, line)
				continue
			}
			if value, ok := updates[key]; ok {
				lines = append(lines, key+"="+value)
				seen[key] = true
				continue
			}
			lines = append(lines, line)
		}
		if err := scanner.Err(); err != nil {
			_ = file.Close()
			return err
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
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

func readEnvValue(path, target string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if ok && strings.TrimSpace(key) == target {
			return strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	return ""
}

func runCommand(dir string, name string, args ...string) error {
	_, err := runCommandOutput(dir, name, args...)
	return err
}

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
