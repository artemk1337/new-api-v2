package service

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

type systemUpdateReleasePage struct {
	latestTag string
	page      int
	pageSize  int
	total     int
	releases  []SystemUpdateRelease
}

type githubReleaseNotes struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
}

func fetchSystemUpdateReleasePage(ctx context.Context, currentVersion string, page, pageSize int) (systemUpdateReleasePage, error) {
	result := systemUpdateReleasePage{page: max(1, page), pageSize: min(10, max(1, pageSize))}
	refs, err := fetchSystemUpdateTagRefs(ctx)
	if err != nil {
		return result, err
	}
	tags := make([]string, 0, len(refs))
	for _, ref := range refs {
		tag := strings.TrimPrefix(ref.Ref, "refs/tags/")
		if isStableVersionTag(tag) {
			tags = append(tags, tag)
		}
	}
	if len(tags) == 0 {
		result.page = 1
		return result, nil
	}
	slices.SortFunc(tags, compareStableVersionTags)
	result.latestTag = tags[len(tags)-1]
	if isStableVersionTag(currentVersion) {
		tags = slices.DeleteFunc(tags, func(tag string) bool {
			return compareStableVersionTags(tag, currentVersion) == 0
		})
	}
	result.total = len(tags)
	result.page = min(result.page, max(1, (result.total+result.pageSize-1)/result.pageSize))
	end := max(0, result.total-(result.page-1)*result.pageSize)
	start := max(0, end-result.pageSize)
	result.releases = make([]SystemUpdateRelease, end-start)
	var requests sync.WaitGroup
	for index, tag := range tags[start:end] {
		result.releases[index] = *buildSystemUpdateTagRelease(tag)
		requests.Go(func() {
			if notes, err := fetchSystemUpdateReleaseNotes(ctx, tag); err == nil {
				release := &result.releases[index]
				release.Name = notes.Name
				release.Body = notes.Body
				release.HTMLURL = notes.HTMLURL
				release.PublishedAt = notes.PublishedAt
			}
		})
	}
	requests.Wait()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	for _, release := range result.releases {
		if strings.TrimSpace(release.Body) == "" {
			sections, err := fetchSystemUpdateChangelogSections(ctx, result.latestTag)
			if err == nil {
				for index := range result.releases {
					if strings.TrimSpace(result.releases[index].Body) == "" {
						result.releases[index].Body = sections[result.releases[index].TagName]
					}
				}
			}
			break
		}
	}
	return result, ctx.Err()
}

func fetchSystemUpdateReleaseNotes(ctx context.Context, tag string) (githubReleaseNotes, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", systemUpdateRepository(), tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return githubReleaseNotes{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "new-api-system-update")
	resp, err := systemUpdateHTTPClient.Do(req)
	if err != nil {
		return githubReleaseNotes{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return githubReleaseNotes{}, fmt.Errorf("github releases api returned status %d", resp.StatusCode)
	}
	notes := githubReleaseNotes{}
	if err := common.DecodeJson(resp.Body, &notes); err != nil {
		return githubReleaseNotes{}, err
	}
	if notes.TagName != tag || notes.Draft || notes.Prerelease {
		return githubReleaseNotes{}, fmt.Errorf("github release does not match stable tag %s", tag)
	}
	return notes, nil
}
