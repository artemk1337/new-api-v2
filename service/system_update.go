package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	defaultUpdateRepository  = "artemk1337/new-api-v2"
	defaultUpdateSidecarURL  = "http://new-api-updater:18090"
	systemUpdatePollInterval = 3 * time.Second
	systemUpdateMaxWait      = 30 * time.Minute
)

type SystemUpdateRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name,omitempty"`
	Body        string `json:"body,omitempty"`
	HTMLURL     string `json:"html_url,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
}

type SystemUpdateCheckResult struct {
	Enabled         bool                 `json:"enabled"`
	Repository      string               `json:"repository"`
	CurrentVersion  string               `json:"current_version"`
	LatestVersion   string               `json:"latest_version"`
	UpdateAvailable bool                 `json:"update_available"`
	Release         *SystemUpdateRelease `json:"release,omitempty"`
}

type SystemUpdatePayload struct {
	Version string `json:"version"`
}

type SystemUpdateState struct {
	Step     string `json:"step"`
	Progress int    `json:"progress"`
	Message  string `json:"message,omitempty"`
}

type SystemUpdateResult struct {
	PreviousVersion  string `json:"previous_version"`
	RequestedVersion string `json:"requested_version"`
	Image            string `json:"image"`
	JobID            string `json:"job_id"`
	Status           string `json:"status"`
}

type systemUpdaterRequest struct {
	Tag string `json:"tag"`
}

type systemUpdaterResponse struct {
	Accepted bool   `json:"accepted"`
	JobID    string `json:"job_id,omitempty"`
	Image    string `json:"image,omitempty"`
	Message  string `json:"message"`
}

type SystemUpdaterJobStatus struct {
	JobID   string `json:"job_id"`
	Status  string `json:"status"`
	Step    string `json:"step"`
	Image   string `json:"image,omitempty"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

type githubTagRef struct {
	Ref string `json:"ref"`
}

var (
	systemUpdateHTTPClient = &http.Client{Timeout: 15 * time.Second}
	stableVersionTagRe     = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.([0-9]+)$`)
)

func CheckSystemUpdate(ctx context.Context) (*SystemUpdateCheckResult, error) {
	result := &SystemUpdateCheckResult{
		Enabled:        systemUpdateEnabled(),
		Repository:     systemUpdateRepository(),
		CurrentVersion: common.Version,
	}
	if !result.Enabled {
		return result, nil
	}

	release, err := fetchLatestSystemUpdateTag(ctx)
	if err != nil {
		return nil, err
	}
	result.LatestVersion = release.TagName
	result.UpdateAvailable = release.TagName != "" && release.TagName != common.Version
	result.Release = release
	return result, nil
}

func StartSystemUpdateTask(version string) (*model.SystemTask, bool, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		return nil, false, errors.New("version is required")
	}
	return EnqueueSystemTask(model.SystemTaskTypeSystemUpdate, SystemUpdatePayload{Version: version})
}

func RunSystemUpdateTask(ctx context.Context, task *model.SystemTask, runnerID string) error {
	payload := SystemUpdatePayload{}
	if err := task.DecodePayload(&payload); err != nil {
		return err
	}
	payload.Version = strings.TrimSpace(payload.Version)
	if payload.Version == "" {
		return errors.New("version is required")
	}
	if !systemUpdateEnabled() {
		return errors.New("system update is disabled")
	}

	if err := updateSystemUpdateState(task, runnerID, "checking", 10, "validating update tag"); err != nil {
		return err
	}
	release, err := fetchSystemUpdateTag(ctx, payload.Version)
	if err != nil {
		return err
	}
	if release.TagName != payload.Version {
		return fmt.Errorf("update tag mismatch: requested %s, got %s", payload.Version, release.TagName)
	}
	if payload.Version == common.Version {
		return errors.New("requested version is already running")
	}

	if err := updateSystemUpdateState(task, runnerID, "requesting_updater", 60, "requesting updater sidecar"); err != nil {
		return err
	}
	updaterJob, err := requestSystemUpdater(ctx, payload.Version)
	if err != nil {
		return err
	}
	if err := updateSystemUpdateState(task, runnerID, "pulling", 40, "pulling update image"); err != nil {
		return err
	}
	updaterStatus, err := waitSystemUpdaterJob(ctx, updaterJob.JobID)
	if err != nil {
		return err
	}

	if err := updateSystemUpdateState(task, runnerID, updaterStatus.Step, 100, updaterStatus.Message); err != nil {
		return err
	}
	result := SystemUpdateResult{
		PreviousVersion:  common.Version,
		RequestedVersion: payload.Version,
		Image:            updaterStatus.Image,
		JobID:            updaterJob.JobID,
		Status:           updaterStatus.Status,
	}
	return model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, result, "")
}

func updateSystemUpdateState(task *model.SystemTask, runnerID string, step string, progress int, message string) error {
	return model.UpdateSystemTaskState(task.TaskID, runnerID, SystemUpdateState{
		Step:     step,
		Progress: progress,
		Message:  message,
	})
}

func fetchLatestSystemUpdateTag(ctx context.Context) (*SystemUpdateRelease, error) {
	refs, err := fetchSystemUpdateTagRefs(ctx)
	if err != nil {
		return nil, err
	}
	latest := ""
	for _, ref := range refs {
		tag := strings.TrimPrefix(ref.Ref, "refs/tags/")
		if !isStableVersionTag(tag) {
			continue
		}
		if latest == "" || compareStableVersionTags(tag, latest) > 0 {
			latest = tag
		}
	}
	if latest == "" {
		return nil, errors.New("github tags payload has no stable version tags")
	}
	return buildSystemUpdateTagRelease(latest), nil
}

func fetchSystemUpdateTag(ctx context.Context, tag string) (*SystemUpdateRelease, error) {
	repository := systemUpdateRepository()
	if repository == "" || !strings.Contains(repository, "/") {
		return nil, errors.New("update repository must be in owner/repo format")
	}
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil, errors.New("update tag is required")
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/git/ref/tags/%s", repository, tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "new-api-system-update")

	resp, err := systemUpdateHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github tags api returned status %d", resp.StatusCode)
	}

	ref := githubTagRef{}
	if err := common.DecodeJson(resp.Body, &ref); err != nil {
		return nil, err
	}
	if strings.TrimPrefix(ref.Ref, "refs/tags/") != tag {
		return nil, errors.New("github tag payload has unexpected ref")
	}
	return buildSystemUpdateTagRelease(tag), nil
}

func fetchSystemUpdateTagRefs(ctx context.Context) ([]githubTagRef, error) {
	repository := systemUpdateRepository()
	if repository == "" || !strings.Contains(repository, "/") {
		return nil, errors.New("update repository must be in owner/repo format")
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/git/matching-refs/tags/", repository)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "new-api-system-update")

	resp, err := systemUpdateHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github tags api returned status %d", resp.StatusCode)
	}

	refs := []githubTagRef{}
	if err := common.DecodeJson(resp.Body, &refs); err != nil {
		return nil, err
	}
	return refs, nil
}

func buildSystemUpdateTagRelease(tag string) *SystemUpdateRelease {
	repository := systemUpdateRepository()
	return &SystemUpdateRelease{
		TagName: tag,
		Name:    tag,
		HTMLURL: fmt.Sprintf("https://github.com/%s/tree/%s", repository, tag),
	}
}

func isStableVersionTag(tag string) bool {
	return stableVersionTagRe.MatchString(tag)
}

func compareStableVersionTags(left string, right string) int {
	leftParts := stableVersionTagParts(left)
	rightParts := stableVersionTagParts(right)
	for i := range leftParts {
		if leftParts[i] > rightParts[i] {
			return 1
		}
		if leftParts[i] < rightParts[i] {
			return -1
		}
	}
	return 0
}

func stableVersionTagParts(tag string) [3]int {
	match := stableVersionTagRe.FindStringSubmatch(tag)
	if match == nil {
		return [3]int{}
	}
	var parts [3]int
	for i := range parts {
		value, _ := strconv.Atoi(match[i+1])
		parts[i] = value
	}
	return parts
}

func requestSystemUpdater(ctx context.Context, version string) (*systemUpdaterResponse, error) {
	token := systemUpdateSidecarToken()
	if token == "" {
		return nil, errors.New("update sidecar token is required")
	}
	body, err := common.Marshal(systemUpdaterRequest{
		Tag: version,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(systemUpdateSidecarURL(), "/")+"/update", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := systemUpdateHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("updater sidecar returned status %d", resp.StatusCode)
	}

	updaterResp := systemUpdaterResponse{}
	if err := common.DecodeJson(resp.Body, &updaterResp); err != nil {
		return nil, err
	}
	if !updaterResp.Accepted {
		if updaterResp.Message == "" {
			updaterResp.Message = "updater sidecar rejected request"
		}
		return nil, errors.New(updaterResp.Message)
	}
	if updaterResp.JobID == "" {
		return nil, errors.New("updater sidecar response has no job_id")
	}
	return &updaterResp, nil
}

func waitSystemUpdaterJob(ctx context.Context, jobID string) (*SystemUpdaterJobStatus, error) {
	deadline := time.NewTimer(systemUpdateMaxWait)
	defer deadline.Stop()

	ticker := time.NewTicker(systemUpdatePollInterval)
	defer ticker.Stop()

	for {
		status, err := getSystemUpdaterJobStatus(ctx, jobID)
		if err != nil {
			return nil, err
		}
		switch status.Status {
		case "succeeded":
			if status.Message == "" {
				status.Message = "update deployed"
			}
			return status, nil
		case "deploying":
			if status.Message == "" {
				status.Message = "update image pulled; deploying service"
			}
			return status, nil
		case "failed":
			if status.Error == "" {
				status.Error = "updater sidecar job failed"
			}
			return nil, errors.New(status.Error)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, errors.New("updater sidecar job timed out")
		case <-ticker.C:
		}
	}
}

func getSystemUpdaterJobStatus(ctx context.Context, jobID string) (*SystemUpdaterJobStatus, error) {
	token := systemUpdateSidecarToken()
	if token == "" {
		return nil, errors.New("update sidecar token is required")
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, errors.New("updater sidecar job id is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(systemUpdateSidecarURL(), "/")+"/jobs/"+jobID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := systemUpdateHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("updater sidecar job status returned status %d", resp.StatusCode)
	}
	status := &SystemUpdaterJobStatus{}
	if err := common.DecodeJson(resp.Body, status); err != nil {
		return nil, err
	}
	if status.JobID == "" {
		return nil, errors.New("updater sidecar job status has no job_id")
	}
	return status, nil
}

func GetSystemUpdaterJobStatus(ctx context.Context, jobID string) (*SystemUpdaterJobStatus, error) {
	return getSystemUpdaterJobStatus(ctx, jobID)
}

func systemUpdateEnabled() bool {
	return common.GetEnvOrDefaultBool("UPDATE_ENABLED", false)
}

func systemUpdateRepository() string {
	return common.GetEnvOrDefaultString("UPDATE_CHECK_REPOSITORY", defaultUpdateRepository)
}

func systemUpdateSidecarURL() string {
	return common.GetEnvOrDefaultString("UPDATE_SIDECAR_URL", defaultUpdateSidecarURL)
}

func systemUpdateSidecarToken() string {
	return strings.TrimSpace(common.GetEnvOrDefaultString("UPDATE_SIDECAR_TOKEN", ""))
}
