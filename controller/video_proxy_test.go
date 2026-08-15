package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoProxyForwardsRangeAndWritesPartialResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "bytes=10-19", r.Header.Get("Range"))
		assert.Equal(t, "video-tag", r.Header.Get("If-Range"))
		assert.Empty(t, r.Header.Get("Connection"))
		w.Header().Set("Content-Range", "bytes 10-19/100")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Connection", "close")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("0123456789"))
	}))
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodGet, upstream.URL, nil)
	req.Header.Set("Range", "bytes=10-19")
	req.Header.Set("If-Range", "video-tag")
	proxyReq, err := http.NewRequestWithContext(req.Context(), http.MethodGet, upstream.URL, nil)
	require.NoError(t, err)
	forwardVideoRequestHeaders(req.Header, proxyReq.Header)
	resp, err := http.DefaultClient.Do(proxyReq)
	require.NoError(t, err)
	defer resp.Body.Close()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = req
	require.NoError(t, writeVideoResponse(c, resp))

	assert.Equal(t, http.StatusPartialContent, recorder.Code)
	assert.Equal(t, "bytes 10-19/100", recorder.Header().Get("Content-Range"))
	assert.Equal(t, "bytes", recorder.Header().Get("Accept-Ranges"))
	assert.Empty(t, recorder.Header().Get("Connection"))
	body, err := io.ReadAll(recorder.Body)
	require.NoError(t, err)
	assert.Equal(t, "0123456789", string(body))
}

func TestForwardVideoRequestHeadersDoesNotForwardHopByHop(t *testing.T) {
	src := make(http.Header)
	src.Set("Range", "bytes=0-")
	src.Set("If-Range", "etag")
	src.Set("Connection", "close")
	dst := make(http.Header)

	forwardVideoRequestHeaders(src, dst)

	assert.Equal(t, "bytes=0-", dst.Get("Range"))
	assert.Equal(t, "etag", dst.Get("If-Range"))
	assert.Empty(t, dst.Get("Connection"))
}

func TestWriteVideoResponseFiltersConnectionTokens(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Connection":     {"close, X-Upstream-Hop"},
			"X-Upstream-Hop": {"secret"},
			"X-Video":        {"ok"},
		},
		Body: io.NopCloser(strings.NewReader("video")),
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	require.NoError(t, writeVideoResponse(c, resp))
	assert.Empty(t, recorder.Header().Get("X-Upstream-Hop"))
	assert.Equal(t, "ok", recorder.Header().Get("X-Video"))
}
