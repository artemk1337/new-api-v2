package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTurnstileCheckOnceDoesNotReuseSessionApproval(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var verifications atomic.Int64
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		verifications.Add(1)
		return turnstileResponse(`{"success":true,"action":"signup"}`), nil
	})}

	previousClient := turnstileHTTPClient
	previousEnabled := common.TurnstileCheckEnabled
	turnstileHTTPClient = client
	common.TurnstileCheckEnabled = true
	t.Cleanup(func() {
		turnstileHTTPClient = previousClient
		common.TurnstileCheckEnabled = previousEnabled
	})

	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.GET("/register", TurnstileCheckOnce("signup"), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodGet, "/register?turnstile=token", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusNoContent, response.Code)

	request = httptest.NewRequest(http.MethodGet, "/register", nil)
	for _, cookie := range response.Result().Cookies() {
		request.AddCookie(cookie)
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"success":false`)
	assert.Equal(t, int64(1), verifications.Load())
}

func TestTurnstileCheckOnceAcceptsHeaderWithoutQueryToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var requestForm url.Values
	var parseErr error
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		parseErr = r.ParseForm()
		requestForm = r.PostForm
		return turnstileResponse(`{"success":true,"action":"signup"}`), nil
	})}

	previousClient := turnstileHTTPClient
	previousEnabled := common.TurnstileCheckEnabled
	previousSecret := common.TurnstileSecretKey
	turnstileHTTPClient = client
	common.TurnstileCheckEnabled = true
	common.TurnstileSecretKey = "secret"
	t.Cleanup(func() {
		turnstileHTTPClient = previousClient
		common.TurnstileCheckEnabled = previousEnabled
		common.TurnstileSecretKey = previousSecret
	})

	router := gin.New()
	router.GET("/verification", TurnstileCheckOnce("signup"), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodGet, "/verification", nil)
	request.Header.Set("X-Turnstile-Token", "header-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusNoContent, response.Code)
	require.NoError(t, parseErr)
	assert.Equal(t, "header-token", requestForm.Get("response"))
	assert.Equal(t, "secret", requestForm.Get("secret"))
}

func TestTurnstileCheckOnceRejectsTokenFromAnotherAction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return turnstileResponse(`{"success":true,"action":"login"}`), nil
	})}

	previousClient := turnstileHTTPClient
	previousEnabled := common.TurnstileCheckEnabled
	turnstileHTTPClient = client
	common.TurnstileCheckEnabled = true
	t.Cleanup(func() {
		turnstileHTTPClient = previousClient
		common.TurnstileCheckEnabled = previousEnabled
	})

	router := gin.New()
	router.GET("/register", TurnstileCheckOnce("signup"), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodGet, "/register?turnstile=token", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"success":false`)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func turnstileResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
