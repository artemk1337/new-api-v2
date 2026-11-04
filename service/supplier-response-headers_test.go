package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSupplierUpstreamHeadersUsePublicAllowlist(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("supplier_pricing", true)
	for _, header := range []string{"Server", "Set-Cookie", "Content-Disposition", "X-Provider", "Via", "X-Request-Id", "OpenAI-Organization", "X-RateLimit-Remaining-Credits", "X-New-Private-Field"} {
		require.False(t, ShouldCopyUpstreamHeader(c, header, []string{"private-source"}), header)
	}
	for _, header := range []string{"Content-Type", "Cache-Control", "Retry-After", "Expires", "Pragma"} {
		require.True(t, ShouldCopyUpstreamHeader(c, header, []string{"value"}), header)
	}
	require.False(t, ShouldCopyUpstreamHeader(c, common.RequestIdKey, []string{"private-request-id"}))
	require.Equal(t, "private-request-id", c.GetString(common.UpstreamRequestIdKey))
	require.False(t, ShouldCopyUpstreamHeader(c, "Content-Length", []string{"1000"}))
	c.Set("supplier_pricing", false)
	require.True(t, ShouldCopyUpstreamHeader(c, "X-Provider", []string{"legacy"}))
	require.True(t, ShouldCopyUpstreamHeader(nil, "X-Provider", []string{"legacy"}))
}
