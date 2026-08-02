package main

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type updateResponse struct {
	Accepted bool   `json:"accepted"`
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

type telemetryAgentResponse struct {
	Running bool   `json:"running"`
	Message string `json:"message"`
}

const manualUpdateMessage = "automatic updates are disabled; run ./install.sh update <tag> on the Docker host"

var (
	tagPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	runCommandFn = runCommand
)

var runCommandOutputFn = runCommandOutput

func main() {
	addr := env("UPDATER_ADDR", ":18090")
	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	http.HandleFunc("/update", handleUpdate)
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
	writeJSON(w, http.StatusServiceUnavailable, updateResponse{Message: manualUpdateMessage})
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
		versions = append(versions, versionReadiness{Tag: tag, Status: "unavailable", Error: manualUpdateMessage})
	}
	writeAnyJSON(w, http.StatusOK, versionReadinessResponse{Versions: versions})
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
