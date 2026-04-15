package netutil

import (
	"net"
	"net/http"
	"strings"
)

// trustedProxies holds the set of trusted proxy networks. When nil, no proxies are trusted.
var trustedProxies []*net.IPNet

// SetTrustedProxies configures which proxy IPs/CIDRs are trusted for X-Forwarded-For parsing.
// Supports both individual IPs ("10.0.0.1") and CIDR ranges ("10.0.0.0/8", "172.16.0.0/12").
// When the list is empty (default), X-Forwarded-For and X-Real-IP are ignored — RemoteAddr is used.
func SetTrustedProxies(proxies []string) {
	if len(proxies) == 0 {
		trustedProxies = nil
		return
	}
	trustedProxies = make([]*net.IPNet, 0, len(proxies))
	for _, p := range proxies {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(p, "/") {
			_, cidr, err := net.ParseCIDR(p)
			if err != nil {
				continue
			}
			trustedProxies = append(trustedProxies, cidr)
		} else {
			ip := net.ParseIP(p)
			if ip == nil {
				continue
			}
			// Store as /32 or /128
			if ip.To4() != nil {
				_, cidr, _ := net.ParseCIDR(p + "/32")
				trustedProxies = append(trustedProxies, cidr)
			} else {
				_, cidr, _ := net.ParseCIDR(p + "/128")
				trustedProxies = append(trustedProxies, cidr)
			}
		}
	}
}

// isTrustedProxy checks if an IP is in the trusted proxy list.
func isTrustedProxy(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, cidr := range trustedProxies {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// RemoteAddrIP strips the port from a RemoteAddr and normalizes IPv6 brackets.
func RemoteAddrIP(remoteAddr string) string {
	if idx := strings.LastIndex(remoteAddr, ":"); idx != -1 {
		// Check for IPv6 like [::1]:8080
		if strings.Contains(remoteAddr, "[") {
			if closeIdx := strings.Index(remoteAddr, "]"); closeIdx != -1 && closeIdx < idx {
				return remoteAddr[1:closeIdx]
			}
		}
		remoteAddr = remoteAddr[:idx]
	}
	return strings.Trim(remoteAddr, "[]")
}

// ExtractClientIP extracts the real client IP from the request.
//
// When trusted_proxies is configured:
//   - Parses X-Forwarded-For right-to-left, skipping trusted proxy IPs
//   - Returns the rightmost untrusted IP (the last non-proxy hop)
//   - Only trusts X-Real-IP from a known trusted proxy
//
// When no trusted proxies are configured (default):
//   - X-Forwarded-For and X-Real-IP are IGNORED (secure by default)
//   - Returns RemoteAddr only
//
// This prevents client IP spoofing when the gateway is directly exposed
// to the internet without a trusted reverse proxy in front.
func ExtractClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	remoteIP := RemoteAddrIP(r.RemoteAddr)

	// Secure by default: if no trusted proxies configured, ignore forwarding headers
	if len(trustedProxies) == 0 {
		return remoteIP
	}

	// Only trust forwarding headers if the immediate connection is from a trusted proxy
	if !isTrustedProxy(remoteIP) {
		return remoteIP
	}

	// Right-to-left X-Forwarded-For parsing
	// XFF format: "client, proxy1, proxy2, ..."
	// We walk from right to left, skipping trusted proxies, until we find the client
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		entries := strings.Split(xff, ",")
		for i := len(entries) - 1; i >= 0; i-- {
			ip := strings.TrimSpace(entries[i])
			if ip == "" {
				continue
			}
			// If this entry is a trusted proxy, continue walking left
			if isTrustedProxy(ip) {
				continue
			}
			// Found the rightmost untrusted IP — this is the client
			return ip
		}
	}

	// Fall back to X-Real-IP (only trusted because we already verified remoteIP is trusted)
	xri := r.Header.Get("X-Real-Ip")
	if xri != "" {
		// M-003: Validate X-Real-IP is a valid IP before using it.
		// A malicious or compromised trusted proxy could spoof this header
		// to bypass IP-based access controls. Validate format first.
		trimmed := strings.TrimSpace(xri)
		if net.ParseIP(trimmed) != nil {
			return trimmed
		}
		// Invalid X-Real-IP format — fall back to remoteAddr
	}

	return remoteIP
}
