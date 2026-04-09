// Package loadbalancer provides load balancing algorithms.
package loadbalancer

import (
	"net"
	"strings"
)

// SubnetResolver resolves IP addresses to subnet groups based on their
// first two octets. For production geographic routing, integrate
// MaxMind GeoIP2 or similar.
type SubnetResolver struct {
	groups map[string]string // IP prefix -> group code
}

// NewSubnetResolver creates a new subnet resolver.
func NewSubnetResolver() *SubnetResolver {
	return &SubnetResolver{
		groups: loadDefaultSubnetData(),
	}
}

// Resolve resolves an IP address to a group code based on its first two octets.
func (s *SubnetResolver) Resolve(ip string) string {
	// Parse IP
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return "UNKNOWN"
	}

	// For IPv4, extract first 2 octets
	if parsedIP.To4() != nil {
		prefix := parsedIP.String()[:strings.LastIndex(parsedIP.String(), ".")]
		prefix = prefix[:strings.LastIndex(prefix, ".")]
		if group, ok := s.groups[prefix]; ok {
			return group
		}
	}

	return "UNKNOWN"
}

// SubnetAwareSelector selects targets based on subnet proximity.
type SubnetAwareSelector struct {
	resolver *SubnetResolver
	// Target locations: target ID -> group code
	targetLocations map[string]string
}

// NewSubnetAwareSelector creates a new subnet-aware selector.
func NewSubnetAwareSelector() *SubnetAwareSelector {
	return &SubnetAwareSelector{
		resolver:        NewSubnetResolver(),
		targetLocations: make(map[string]string),
	}
}

// SetTargetLocation sets the location for a target.
func (s *SubnetAwareSelector) SetTargetLocation(targetID, countryCode string) {
	s.targetLocations[targetID] = countryCode
}

// Select selects the closest target based on client IP.
func (s *SubnetAwareSelector) Select(clientIP string, targetIDs []string) string {
	if len(targetIDs) == 0 {
		return ""
	}

	clientGroup := s.resolver.Resolve(clientIP)

	// Find targets in the same group
	for _, id := range targetIDs {
		if location, ok := s.targetLocations[id]; ok && location == clientGroup {
			return id
		}
	}

	// Fall back to first target
	return targetIDs[0]
}

// loadDefaultSubnetData loads bundled subnet prefix-to-group mappings.
func loadDefaultSubnetData() map[string]string {
	return map[string]string{
		"192.168": "US",
		"10.0":    "US",
		"172.16":  "EU",
		"127.0":   "LOCAL",
	}
}
