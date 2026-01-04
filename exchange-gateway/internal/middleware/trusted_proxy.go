package middleware

import (
	"fmt"
	"net"
	"strings"
)

var (
	trustedProxyCIDRs       []*net.IPNet
	useExplicitTrustedCIDRs bool
)

// SetTrustedProxyCIDRs sets explicit trusted proxy CIDRs. When non-empty, only
// requests coming from these CIDRs will have X-Forwarded-For trusted.
func SetTrustedProxyCIDRs(cidrs []string) error {
	trustedProxyCIDRs = nil
	useExplicitTrustedCIDRs = false
	for _, raw := range cidrs {
		cidr := strings.TrimSpace(raw)
		if cidr == "" {
			continue
		}
		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			return fmt.Errorf("invalid trusted proxy CIDR %q", cidr)
		}
		trustedProxyCIDRs = append(trustedProxyCIDRs, ipnet)
	}
	if len(trustedProxyCIDRs) > 0 {
		useExplicitTrustedCIDRs = true
	}
	return nil
}

// IsTrustedProxyIP returns true if the IP should be treated as a trusted proxy.
// Default (no explicit CIDRs): loopback or private IPs are trusted.
func IsTrustedProxyIP(ipStr string) bool {
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return false
	}
	if useExplicitTrustedCIDRs {
		for _, ipnet := range trustedProxyCIDRs {
			if ipnet.Contains(ip) {
				return true
			}
		}
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}
