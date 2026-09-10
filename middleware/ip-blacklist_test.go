package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIPBlacklistBlocksBeforeNextHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	settings := system_setting.GetSecuritySettings()
	original := append([]string(nil), settings.IPBlacklist...)
	t.Cleanup(func() {
		value, _ := common.Marshal(original)
		_ = settings.UpdateConfigFromMap(map[string]string{"ip_blacklist": string(value)})
	})
	require.NoError(t, settings.UpdateConfigFromMap(map[string]string{
		"ip_blacklist": `["203.0.113.7"]`,
	}))

	server := gin.New()
	require.NoError(t, server.SetTrustedProxies(nil))
	called := false
	server.Use(IPBlacklist())
	server.GET("/", func(c *gin.Context) {
		called = true
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "203.0.113.7:12345"
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.False(t, called)
}

func TestIPBlacklistUsesGinClientIPWithoutUntrustedForwardedHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	settings := system_setting.GetSecuritySettings()
	original := append([]string(nil), settings.IPBlacklist...)
	t.Cleanup(func() {
		value, _ := common.Marshal(original)
		_ = settings.UpdateConfigFromMap(map[string]string{"ip_blacklist": string(value)})
	})
	require.NoError(t, settings.UpdateConfigFromMap(map[string]string{
		"ip_blacklist": `["198.51.100.10"]`,
	}))

	server := gin.New()
	require.NoError(t, server.SetTrustedProxies(nil))
	server.Use(IPBlacklist())
	server.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "203.0.113.7:12345"
	request.Header.Set("X-Forwarded-For", "198.51.100.10")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNoContent, response.Code)
}
