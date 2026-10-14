package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemUpdateReleaseNotesPreferPublishedRelease(t *testing.T) {
	t.Setenv("UPDATE_CHECK_REPOSITORY", "artemk1337/new-api-v2")
	withSystemUpdateHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host + req.URL.Path {
		case "api.github.com/repos/artemk1337/new-api-v2/git/matching-refs/tags/":
			return jsonResponse(http.StatusOK, `[{"ref":"refs/tags/v1.0.1"}]`), nil
		case "api.github.com/repos/artemk1337/new-api-v2/releases/tags/v1.0.1":
			return jsonResponse(http.StatusOK, `{"tag_name":"v1.0.1","name":"Release title","body":"## Changes\n\n- Published notes","html_url":"https://github.com/artemk1337/new-api-v2/releases/tag/v1.0.1","published_at":"2026-10-15T12:00:00Z"}`), nil
		default:
			t.Errorf("unexpected request: %s", req.URL)
			return jsonResponse(http.StatusNotFound, `{}`), nil
		}
	})

	page, err := fetchSystemUpdateReleasePage(t.Context(), "v1.0.0", 1, 5)
	require.NoError(t, err)
	require.Len(t, page.releases, 1)
	release := page.releases[0]
	assert.Equal(t, "Release title", release.Name)
	assert.Equal(t, "## Changes\n\n- Published notes", release.Body)
	assert.Equal(t, "https://github.com/artemk1337/new-api-v2/releases/tag/v1.0.1", release.HTMLURL)
	assert.Equal(t, "2026-10-15T12:00:00Z", release.PublishedAt)
}

func TestSystemUpdateReleaseNotesFallbackToDatedChangelog(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		body   string
	}{
		{"missing", http.StatusNotFound, `{"message":"Not Found"}`},
		{"empty", http.StatusOK, `{"tag_name":"v1.0.1","body":"  \n"}`},
		{"api error", http.StatusForbidden, `{"message":"API rate limit exceeded"}`},
		{"draft", http.StatusOK, `{"tag_name":"v1.0.1","draft":true,"body":"Unpublished"}`},
		{"prerelease", http.StatusOK, `{"tag_name":"v1.0.1","prerelease":true,"body":"Preview"}`},
		{"wrong tag", http.StatusOK, `{"tag_name":"v1.0.2","body":"Wrong release"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("UPDATE_CHECK_REPOSITORY", "artemk1337/new-api-v2")
			fallbackRequests := 0
			withSystemUpdateHTTPClient(t, func(req *http.Request) (*http.Response, error) {
				switch req.URL.Host + req.URL.Path {
				case "api.github.com/repos/artemk1337/new-api-v2/git/matching-refs/tags/":
					return jsonResponse(http.StatusOK, `[{"ref":"refs/tags/v1.0.1"}]`), nil
				case "api.github.com/repos/artemk1337/new-api-v2/releases/tags/v1.0.1":
					return jsonResponse(tt.status, tt.body), nil
				case "raw.githubusercontent.com/artemk1337/new-api-v2/v1.0.1/CHANGELOG.md":
					fallbackRequests++
					return jsonResponse(http.StatusOK, "# Changelog\n\n## v1.0.1 — 2026-10-15\n\n- Fallback notes\n\n## v1.0.0\n\n- Older\n"), nil
				default:
					t.Errorf("unexpected request: %s", req.URL)
					return jsonResponse(http.StatusNotFound, `{}`), nil
				}
			})

			page, err := fetchSystemUpdateReleasePage(t.Context(), "v1.0.0", 1, 5)
			require.NoError(t, err)
			require.Len(t, page.releases, 1)
			assert.Equal(t, "- Fallback notes", page.releases[0].Body)
			assert.Equal(t, 1, fallbackRequests)
		})
	}
}

func TestSystemUpdateReleasePaginationLimitsNotesRequests(t *testing.T) {
	for _, tt := range []struct {
		name                           string
		page, size, wantPage, wantSize int
		wantTags                       []string
	}{
		{"newest", 1, 2, 1, 2, []string{"v1.0.11", "v1.0.12"}},
		{"older", 2, 2, 2, 2, []string{"v1.0.9", "v1.0.10"}},
		{"past end", 99, 2, 6, 2, []string{"v1.0.1"}},
		{"invalid bounds", 0, 0, 1, 1, []string{"v1.0.12"}},
		{"size capped", 1, 100, 1, 10, []string{"v1.0.2", "v1.0.3", "v1.0.4", "v1.0.6", "v1.0.7", "v1.0.8", "v1.0.9", "v1.0.10", "v1.0.11", "v1.0.12"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("UPDATE_CHECK_REPOSITORY", "artemk1337/new-api-v2")
			refs := []string{`{"ref":"refs/tags/v9.0.0-rc.1"}`, `{"ref":"refs/tags/latest"}`}
			for version := 12; version >= 1; version-- {
				refs = append(refs, fmt.Sprintf(`{"ref":"refs/tags/v1.0.%d"}`, version))
			}
			var lock sync.Mutex
			var requestedTags []string
			withSystemUpdateHTTPClient(t, func(req *http.Request) (*http.Response, error) {
				if req.URL.Host == "api.github.com" && req.URL.Path == "/repos/artemk1337/new-api-v2/git/matching-refs/tags/" {
					return jsonResponse(http.StatusOK, "["+strings.Join(refs, ",")+"]"), nil
				}
				if req.URL.Host == "api.github.com" && strings.HasPrefix(req.URL.Path, "/repos/artemk1337/new-api-v2/releases/tags/") {
					tag := strings.TrimPrefix(req.URL.Path, "/repos/artemk1337/new-api-v2/releases/tags/")
					lock.Lock()
					requestedTags = append(requestedTags, tag)
					lock.Unlock()
					return jsonResponse(http.StatusOK, fmt.Sprintf(`{"tag_name":"%s","body":"Published notes"}`, tag)), nil
				}
				t.Errorf("unexpected request: %s", req.URL)
				return jsonResponse(http.StatusNotFound, `{}`), nil
			})

			page, err := fetchSystemUpdateReleasePage(t.Context(), "v1.0.5", tt.page, tt.size)
			require.NoError(t, err)
			assert.Equal(t, "v1.0.12", page.latestTag)
			assert.Equal(t, 11, page.total)
			assert.Equal(t, tt.wantPage, page.page)
			assert.Equal(t, tt.wantSize, page.pageSize)
			var tags []string
			for _, release := range page.releases {
				tags = append(tags, release.TagName)
			}
			assert.Equal(t, tt.wantTags, tags)
			assert.ElementsMatch(t, tt.wantTags, requestedTags)
		})
	}
}

func TestSystemUpdateReleasePaginationEmptyCatalog(t *testing.T) {
	for _, refs := range []string{`[]`, `[{"ref":"refs/tags/v1.0.0"}]`} {
		t.Run(refs, func(t *testing.T) {
			withSystemUpdateHTTPClient(t, func(req *http.Request) (*http.Response, error) {
				assert.True(t, strings.HasSuffix(req.URL.Path, "/git/matching-refs/tags/"))
				return jsonResponse(http.StatusOK, refs), nil
			})
			page, err := fetchSystemUpdateReleasePage(t.Context(), "v1.0.0", 99, 5)
			require.NoError(t, err)
			assert.Empty(t, page.releases)
			assert.Zero(t, page.total)
			assert.Equal(t, 1, page.page)
		})
	}
}

func TestSystemUpdateReleaseNotesHonorCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	withSystemUpdateHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/git/matching-refs/tags/") {
			return jsonResponse(http.StatusOK, `[{"ref":"refs/tags/v1.0.1"}]`), nil
		}
		assert.True(t, strings.HasSuffix(req.URL.Path, "/releases/tags/v1.0.1"))
		cancel()
		return nil, req.Context().Err()
	})
	_, err := fetchSystemUpdateReleasePage(ctx, "v1.0.0", 1, 5)
	require.ErrorIs(t, err, context.Canceled)
}

func TestSystemUpdateReleaseNotesFallbackPreservesPublishedBodies(t *testing.T) {
	t.Setenv("UPDATE_CHECK_REPOSITORY", "artemk1337/new-api-v2")
	fallbackRequests := 0
	withSystemUpdateHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host + req.URL.Path {
		case "api.github.com/repos/artemk1337/new-api-v2/git/matching-refs/tags/":
			return jsonResponse(http.StatusOK, `[{"ref":"refs/tags/v1.0.1"},{"ref":"refs/tags/v1.0.2"},{"ref":"refs/tags/v1.0.3"}]`), nil
		case "api.github.com/repos/artemk1337/new-api-v2/releases/tags/v1.0.1", "api.github.com/repos/artemk1337/new-api-v2/releases/tags/v1.0.3":
			return jsonResponse(http.StatusNotFound, `{"message":"Not Found"}`), nil
		case "api.github.com/repos/artemk1337/new-api-v2/releases/tags/v1.0.2":
			return jsonResponse(http.StatusOK, `{"tag_name":"v1.0.2","body":"Published notes"}`), nil
		case "raw.githubusercontent.com/artemk1337/new-api-v2/v1.0.3/CHANGELOG.md":
			fallbackRequests++
			return jsonResponse(http.StatusOK, "## v1.0.3 — 2026-10-15\n\n- Third\n\n## v1.0.2 — 2026-10-14\n\n- Outdated notes\n\n## v1.0.1 — 2026-10-13\n\n- First\n"), nil
		default:
			t.Errorf("unexpected request: %s", req.URL)
			return jsonResponse(http.StatusNotFound, `{}`), nil
		}
	})

	page, err := fetchSystemUpdateReleasePage(t.Context(), "v1.0.0", 1, 5)
	require.NoError(t, err)
	require.Len(t, page.releases, 3)
	assert.Equal(t, "- First", page.releases[0].Body)
	assert.Equal(t, "Published notes", page.releases[1].Body)
	assert.Equal(t, "- Third", page.releases[2].Body)
	assert.Equal(t, 1, fallbackRequests)
}
