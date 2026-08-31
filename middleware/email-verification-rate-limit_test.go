package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestEmailVerificationRateLimitIsPerEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/verification", EmailVerificationRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	for _, requestURL := range []string{
		"/verification?email=user%40example.com",
		"/verification?email=USER%40EXAMPLE.COM",
		"/verification?email=another%40example.com",
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, requestURL, nil))

		if requestURL == "/verification?email=USER%40EXAMPLE.COM" {
			assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
			continue
		}
		assert.Equal(t, http.StatusNoContent, recorder.Code)
	}
}
