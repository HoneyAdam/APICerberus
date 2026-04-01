package loadbalancer

import (
	"net"
	"strings"
)

// GeoIPResolver resolves IP addresses to geographical locations.
type GeoIPResolver struct {
	// In a production implementation, this would use a GeoIP database
	// like MaxMind GeoIP2 or similar
	countries map[string]string // IP prefix -> country code
}

// NewGeoIPResolver creates a new GeoIP resolver.
func NewGeoIPResolver() *GeoIPResolver {
	return &GeoIPResolver{
		countries: loadDefaultGeoData(),
	}
}

// Resolve resolves an IP address to a country code.
func (g *GeoIPResolver) Resolve(ip string) string {
	// Parse IP
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return "UNKNOWN"
	}

	// For IPv4, extract first 2 octets
	if parsedIP.To4() != nil {
		prefix := parsedIP.String()[:strings.LastIndex(parsedIP.String(), ".")]
		prefix = prefix[:strings.LastIndex(prefix, ".")]
		if country, ok := g.countries[prefix]; ok {
			return country
		}
	}

	return "UNKNOWN"
}

// GeoAwareSelector selects targets based on geographic proximity.
type GeoAwareSelector struct {
	resolver *GeoIPResolver
	// Target locations: target ID -> country code
	targetLocations map[string]string
}

// NewGeoAwareSelector creates a new geo-aware selector.
func NewGeoAwareSelector() *GeoAwareSelector {
	return &GeoAwareSelector{
		resolver:        NewGeoIPResolver(),
		targetLocations: make(map[string]string),
	}
}

// SetTargetLocation sets the location for a target.
func (g *GeoAwareSelector) SetTargetLocation(targetID, countryCode string) {
	g.targetLocations[targetID] = countryCode
}

// Select selects the closest target based on client IP.
func (g *GeoAwareSelector) Select(clientIP string, targetIDs []string) string {
	if len(targetIDs) == 0 {
		return ""
	}

	clientCountry := g.resolver.Resolve(clientIP)

	// Find targets in the same country
	for _, id := range targetIDs {
		if location, ok := g.targetLocations[id]; ok && location == clientCountry {
			return id
		}
	}

	// Fall back to first target
	return targetIDs[0]
}

// loadDefaultGeoData loads bundled GeoIP prefix-to-country mappings.
func loadDefaultGeoData() map[string]string {
	// Simplified mapping of some IP ranges to countries
	return map[string]string{
		"192.168": "US",
		"10.0":    "US",
		"172.16":  "EU",
		"127.0":   "LOCAL",
	}
}
