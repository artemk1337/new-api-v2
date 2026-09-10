package system_setting

import (
	"fmt"
	"net/netip"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

// SecuritySettings contains request admission policies that are controlled by
// administrators through the system options API.
type SecuritySettings struct {
	IPBlacklist []string `json:"ip_blacklist"`
	snapshot    atomic.Pointer[ipBlacklistSnapshot]
}

// IPBlacklistOption is the persisted admin option key for the request
// admission blacklist.
const IPBlacklistOption = "security.ip_blacklist"

var defaultSecuritySettings = SecuritySettings{
	IPBlacklist: []string{},
}

type ipBlacklistSnapshot struct {
	rules []netip.Prefix
}

func init() {
	defaultSecuritySettings.snapshot.Store(&ipBlacklistSnapshot{})
	config.GlobalConfig.Register("security", &defaultSecuritySettings)
}

func GetSecuritySettings() *SecuritySettings {
	return &defaultSecuritySettings
}

// ValidateIPBlacklist checks the JSON value used by the admin option API. Each
// entry may be an IP address or a CIDR prefix. Empty entries are rejected so a
// typo cannot silently disable an intended rule.
func ValidateIPBlacklist(value string) error {
	_, _, err := parseIPBlacklistValue(value)
	return err
}

// UpdateConfigFromMap publishes a new immutable matching snapshot only after
// the complete list has been parsed. ConfigManager calls this method while
// holding its update lock, so readers never observe a partially updated rule
// set or race with the reflective settings update.
func (s *SecuritySettings) UpdateConfigFromMap(configMap map[string]string) error {
	value, ok := configMap["ip_blacklist"]
	if !ok {
		return nil
	}
	entries, rules, err := parseIPBlacklistValue(value)
	if err != nil {
		return err
	}
	s.IPBlacklist = entries
	s.snapshot.Store(&ipBlacklistSnapshot{rules: rules})
	return nil
}

// IsIPBlacklisted reports whether ip matches one of the configured exact IPs
// or CIDR prefixes. Invalid runtime entries are ignored defensively; writes
// through the admin option API are validated before persistence.
func IsIPBlacklisted(ip string) bool {
	address, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return false
	}
	address = canonicalizeIP(address)
	snapshot := defaultSecuritySettings.snapshot.Load()
	if snapshot == nil {
		return false
	}
	for _, prefix := range snapshot.rules {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func parseIPBlacklistEntry(entry string) (netip.Prefix, error) {
	if prefix, err := netip.ParsePrefix(entry); err == nil {
		return canonicalizePrefix(prefix), nil
	}
	address, err := netip.ParseAddr(entry)
	if err != nil {
		return netip.Prefix{}, err
	}
	address = canonicalizeIP(address)
	return netip.PrefixFrom(address, address.BitLen()), nil
}

func parseIPBlacklistValue(value string) ([]string, []netip.Prefix, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed[0] != '[' {
		return nil, nil, fmt.Errorf("ip blacklist must be a JSON array")
	}
	var entries []string
	if err := common.UnmarshalJsonStr(trimmed, &entries); err != nil {
		return nil, nil, fmt.Errorf("ip blacklist must be a JSON array: %w", err)
	}
	rules := make([]netip.Prefix, 0, len(entries))
	for i, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return nil, nil, fmt.Errorf("ip blacklist entry %d is empty", i)
		}
		prefix, err := parseIPBlacklistEntry(entry)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid ip blacklist entry %q: %w", entry, err)
		}
		entries[i] = entry
		rules = append(rules, prefix)
	}
	return entries, rules, nil
}

func canonicalizePrefix(prefix netip.Prefix) netip.Prefix {
	if prefix.Bits() < 96 || !isIPv4Family(prefix.Addr()) {
		return prefix
	}
	return netip.PrefixFrom(canonicalizeIP(prefix.Addr()), prefix.Bits()-96)
}

func canonicalizeIP(address netip.Addr) netip.Addr {
	address = address.Unmap()
	if address.Is4() {
		return address
	}
	if !isIPv4Compatible(address) {
		return address
	}
	bytes := address.As16()
	return netip.AddrFrom4([4]byte{bytes[12], bytes[13], bytes[14], bytes[15]})
}

func isIPv4Family(address netip.Addr) bool {
	if address.Is4In6() {
		return true
	}
	return isIPv4Compatible(address)
}

func isIPv4Compatible(address netip.Addr) bool {
	if !address.Is6() {
		return false
	}
	bytes := address.As16()
	for _, value := range bytes[:12] {
		if value != 0 {
			return false
		}
	}
	// :: and ::1 are IPv6 unspecified/loopback addresses, not deprecated
	// IPv4-compatible encodings.
	last := [4]byte{bytes[12], bytes[13], bytes[14], bytes[15]}
	return last != [4]byte{} && last != [4]byte{0, 0, 0, 1}
}
