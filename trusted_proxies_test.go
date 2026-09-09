package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTrustedProxies(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    []string
		wantErr bool
	}{
		{name: "empty disables proxy trust", value: "", want: nil},
		{name: "whitespace disables proxy trust", value: "  ,  ", want: nil},
		{
			name:  "parses normalizes and deduplicates",
			value: " 127.0.0.1, 10.0.0.7/8, ::1/128, 127.0.0.1 ",
			want:  []string{"127.0.0.1", "10.0.0.0/8", "::1/128"},
		},
		{name: "rejects hostname", value: "proxy.internal", wantErr: true},
		{name: "rejects malformed CIDR", value: "10.0.0.1/99", wantErr: true},
		{name: "rejects all IPv4 addresses", value: "0.0.0.0/0", wantErr: true},
		{name: "rejects all IPv6 addresses", value: "::/0", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTrustedProxies(tt.value)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTrustedProxiesControlClientIP(t *testing.T) {
	tests := []struct {
		name          string
		trusted       string
		remoteAddress string
		forwardedFor  string
		want          string
	}{
		{
			name:          "untrusted peer cannot spoof forwarded header",
			remoteAddress: "203.0.113.7:12345",
			forwardedFor:  "198.51.100.10",
			want:          "203.0.113.7",
		},
		{
			name:          "trusted local proxy forwards client address",
			trusted:       "127.0.0.0/8",
			remoteAddress: "127.0.0.1:12345",
			forwardedFor:  "198.51.100.10",
			want:          "198.51.100.10",
		},
		{
			name:          "chain stops at first untrusted proxy",
			trusted:       "127.0.0.0/8",
			remoteAddress: "127.0.0.1:12345",
			forwardedFor:  "198.51.100.10, 10.0.0.2",
			want:          "10.0.0.2",
		},
		{
			name:          "fully trusted proxy chain forwards client address",
			trusted:       "127.0.0.0/8,10.0.0.0/8",
			remoteAddress: "127.0.0.1:12345",
			forwardedFor:  "198.51.100.10, 10.0.0.2",
			want:          "198.51.100.10",
		},
	}

	gin.SetMode(gin.TestMode)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxies, err := parseTrustedProxies(tt.trusted)
			require.NoError(t, err)

			server := gin.New()
			require.NoError(t, server.SetTrustedProxies(proxies))
			server.GET("/ip", func(c *gin.Context) {
				c.String(http.StatusOK, c.ClientIP())
			})

			request := httptest.NewRequest(http.MethodGet, "/ip", nil)
			request.RemoteAddr = tt.remoteAddress
			request.Header.Set("X-Forwarded-For", tt.forwardedFor)
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)

			require.Equal(t, http.StatusOK, response.Code)
			assert.Equal(t, tt.want, response.Body.String())
		})
	}
}
