package netutil

import (
	"net/http"
	"strings"
)

// trustedProxies holds the set of trusted proxy IPs. When nil/empty, all proxies are trusted.
var trustedProxies map[string]bool

// SetTrustedProxies configures which proxy IPs are trusted for X-Forwarded-For parsing.
// When the list is empty, all forwarding headers are trusted (backward compatible).
func SetTrustedProxies(proxies []string) {
	if len(proxies) == 0 {
		trustedProxies = nil
		return
	}
	trustedProxies = make(map[string]bool, len(proxies))
	for _, p := range proxies {
		p = strings.TrimSpace(p)
		if p != "" {
			trustedProxies[p] = true
		}
	}
}

// RemoteAddrIP strips the port from a RemoteAddr and normalizes IPv6 brackets.
func RemoteAddrIP(remoteAddr string) string {
	if idx := strings.LastIndex(remoteAddr, ":"); idx != -1 {
		remoteAddr = remoteAddr[:idx]
	}
	return strings.Trim(remoteAddr, "[]")
}

// ExtractClientIP extracts the client IP from the request, considering X-Forwarded-For header.
// When trusted_proxies is configured, only parses forwarding headers from trusted sources.
// When empty (default), trusts all forwarding headers for backward compatibility.
func ExtractClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	remoteIP := RemoteAddrIP(r.RemoteAddr)

	trustHeaders := len(trustedProxies) == 0 || trustedProxies[remoteIP]

	if trustHeaders {
		// Check X-Forwarded-For header first (for proxied requests)
		xff := r.Header.Get("X-Forwarded-For")
		if xff != "" {
			ips := strings.Split(xff, ",")
			if len(ips) > 0 {
				clientIP := strings.TrimSpace(ips[0])
				if clientIP != "" {
					return clientIP
				}
			}
		}

		// Fall back to X-Real-Ip header
		xri := r.Header.Get("X-Real-Ip")
		if xri != "" {
			return strings.TrimSpace(xri)
		}
	}

	return remoteIP
}
