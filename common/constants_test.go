package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestThemeAwarePathRedirectsRetiredConsolePaths(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "top up", path: "/console/topup?status=success", want: "/wallet?status=success"},
		{name: "usage logs", path: "/console/log", want: "/usage-logs"},
		{name: "profile", path: "/console/personal/security", want: "/profile/security"},
		{name: "unrelated path", path: "/console/setting", want: "/console/setting"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ThemeAwarePath(tt.path))
		})
	}
}
