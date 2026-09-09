package main

import (
	"fmt"
	"net/netip"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func configureTrustedProxies(server *gin.Engine) error {
	proxies, err := parseTrustedProxies(os.Getenv("TRUSTED_PROXIES"))
	if err != nil {
		return fmt.Errorf("parse TRUSTED_PROXIES: %w", err)
	}
	if err = server.SetTrustedProxies(proxies); err != nil {
		return fmt.Errorf("apply TRUSTED_PROXIES: %w", err)
	}
	return nil
}

func parseTrustedProxies(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}

	parts := strings.Split(value, ",")
	proxies := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		proxy := strings.TrimSpace(part)
		if proxy == "" {
			continue
		}

		if prefix, err := netip.ParsePrefix(proxy); err == nil {
			if prefix.Bits() == 0 {
				return nil, fmt.Errorf("refusing unsafe all-address proxy range %q", proxy)
			}
			proxy = prefix.Masked().String()
		} else if address, err := netip.ParseAddr(proxy); err == nil {
			proxy = address.String()
		} else {
			return nil, fmt.Errorf("invalid IP address or CIDR %q", proxy)
		}

		if _, exists := seen[proxy]; exists {
			continue
		}
		seen[proxy] = struct{}{}
		proxies = append(proxies, proxy)
	}

	if len(proxies) == 0 {
		return nil, nil
	}
	return proxies, nil
}
