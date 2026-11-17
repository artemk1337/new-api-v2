package common

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

func TestEmbeddedAssetsLeaveDirectoriesToFrontend(t *testing.T) {
	assets := &embedFileSystem{FileSystem: http.FS(fstest.MapFS{
		"docs/example.txt": {Data: []byte("asset")},
	})}
	router := gin.New()
	router.Use(static.Serve("/", assets))
	router.NoRoute(func(c *gin.Context) {
		c.String(http.StatusOK, "frontend")
	})
	for _, path := range []string{"/", "/docs", "/docs/", "/docs/?section=guide", "/docs/example.txt"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			want := "frontend"
			if path == "/docs/example.txt" {
				want = "asset"
			}
			if response.Code != http.StatusOK || response.Body.String() != want {
				t.Fatalf("got status %d, body %q; want 200, %q", response.Code, response.Body.String(), want)
			}
		})
	}
}
