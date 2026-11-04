package controller

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
)

const maxSupplierPricingBytes = 16 << 20

func supplierPricingURL(profile model.SupplierPricingProfile) (*url.URL, error) {
	base, err := url.Parse(profile.BaseURL)
	if err != nil || base.Scheme != "https" || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Port() != "" && base.Port() != "443") {
		return nil, fmt.Errorf("supplier pricing requires a public HTTPS origin on port 443")
	}
	endpoint, err := url.Parse(profile.Endpoint)
	if err != nil || endpoint.IsAbs() || endpoint.Host != "" || endpoint.User != nil || endpoint.Fragment != "" || !strings.HasPrefix(profile.Endpoint, "/") || strings.HasPrefix(profile.Endpoint, "//") {
		return nil, fmt.Errorf("supplier pricing endpoint must be an absolute path on the configured origin")
	}
	resolved := base.ResolveReference(endpoint)
	if resolved.Host != base.Host || resolved.Scheme != base.Scheme {
		return nil, fmt.Errorf("supplier pricing endpoint changed origin")
	}
	return resolved, nil
}

func supplierPricingDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port != "443" {
		return nil, fmt.Errorf("supplier pricing requires port 443")
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(addresses) == 0 {
		return nil, fmt.Errorf("supplier pricing hostname could not be resolved")
	}
	protection := common.SSRFProtection{AllowPrivateIp: false, IpFilterMode: false}
	for _, address := range addresses {
		if !protection.IsIPAccessAllowed(address.IP) {
			return nil, fmt.Errorf("supplier pricing resolved to a private or reserved address")
		}
	}
	dialer := net.Dialer{Timeout: 10 * time.Second}
	for _, address := range addresses {
		connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(address.IP.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		err = dialErr
	}
	return nil, err
}

func newSupplierPricingHTTPClient(origin *url.URL) *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           supplierPricingDialContext,
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
			DisableKeepAlives:     true,
		},
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 3 || request.URL.Scheme != origin.Scheme || request.URL.Host != origin.Host || request.URL.User != nil || request.URL.Fragment != "" {
				return fmt.Errorf("supplier pricing redirect left the configured origin")
			}
			return nil
		},
	}
}

func fetchSupplierPricing(ctx context.Context, supplier model.Supplier, profile model.SupplierPricingProfile) ([]types.SupplierModelTariff, error) {
	switch supplier.Adapter {
	case "new-api", "openlux", "bblabu", "relay-fast":
	default:
		return nil, fmt.Errorf("supplier adapter does not support automatic price synchronization")
	}
	if !supplier.Enabled || !profile.Enabled {
		return nil, fmt.Errorf("supplier or pricing profile is disabled")
	}
	target, err := supplierPricingURL(profile)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := newSupplierPricingHTTPClient(target).Do(request)
	if err != nil {
		return nil, fmt.Errorf("supplier pricing fetch failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("supplier pricing returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxSupplierPricingBytes+1))
	if err != nil {
		return nil, fmt.Errorf("supplier pricing response could not be read")
	}
	if len(body) > maxSupplierPricingBytes {
		return nil, fmt.Errorf("supplier pricing response exceeds size limit")
	}
	return normalizeSupplierCatalogue(body, supplier.Adapter, profile)
}
