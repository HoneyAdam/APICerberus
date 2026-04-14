package plugin

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

const (
	ipRestrictModeNone      = "none"
	ipRestrictModeWhitelist = "whitelist"
	ipRestrictModeBlacklist = "blacklist"
)

// IPRestrictConfig configures whitelist/blacklist rules.
type IPRestrictConfig struct {
	Whitelist []string
	Blacklist []string
}

// IPRestrictError indicates request was blocked by IP policy.
type IPRestrictError struct {
	PluginError
}

// IPRestrict plugin blocks requests based on IP and CIDR lists.
type IPRestrict struct {
	mode          string
	whitelistIPs  map[string]struct{}
	whitelistNets []*net.IPNet
	blacklistIPs  map[string]struct{}
	blacklistNets []*net.IPNet
}

func NewIPRestrict(cfg IPRestrictConfig) (*IPRestrict, error) {
	whitelistIPs, whitelistNets, err := parseIPRules(cfg.Whitelist)
	if err != nil {
		return nil, fmt.Errorf("parse whitelist: %w", err)
	}
	blacklistIPs, blacklistNets, err := parseIPRules(cfg.Blacklist)
	if err != nil {
		return nil, fmt.Errorf("parse blacklist: %w", err)
	}

	mode := ipRestrictModeNone
	if len(whitelistIPs) > 0 || len(whitelistNets) > 0 {
		mode = ipRestrictModeWhitelist
	} else if len(blacklistIPs) > 0 || len(blacklistNets) > 0 {
		mode = ipRestrictModeBlacklist
	}

	return &IPRestrict{
		mode:          mode,
		whitelistIPs:  whitelistIPs,
		whitelistNets: whitelistNets,
		blacklistIPs:  blacklistIPs,
		blacklistNets: blacklistNets,
	}, nil
}

func (p *IPRestrict) Name() string  { return "ip-restrict" }
func (p *IPRestrict) Phase() Phase  { return PhasePreAuth }
func (p *IPRestrict) Priority() int { return 5 }

// Allow checks if request IP is allowed. Returns *IPRestrictError when blocked.
func (p *IPRestrict) Allow(req *http.Request) error {
	if p == nil || p.mode == ipRestrictModeNone {
		return nil
	}

	ipValue := requestIP(req)
	ip := net.ParseIP(ipValue)
	if ip == nil {
		return &IPRestrictError{
			PluginError: PluginError{
				Code:    "ip_invalid",
				Message: "Client IP could not be determined",
				Status:  http.StatusForbidden,
			},
		}
	}

	switch p.mode {
	case ipRestrictModeWhitelist:
		if p.matches(ip, p.whitelistIPs, p.whitelistNets) {
			return nil
		}
		return &IPRestrictError{
			PluginError: PluginError{
				Code:    "ip_not_allowed",
				Message: "IP is not in whitelist",
				Status:  http.StatusForbidden,
			},
		}
	case ipRestrictModeBlacklist:
		if p.matches(ip, p.blacklistIPs, p.blacklistNets) {
			return &IPRestrictError{
				PluginError: PluginError{
					Code:    "ip_blocked",
					Message: "IP is blocked",
					Status:  http.StatusForbidden,
				},
			}
		}
		return nil
	default:
		return nil
	}
}

func (p *IPRestrict) matches(ip net.IP, exact map[string]struct{}, cidrs []*net.IPNet) bool {
	if ip == nil {
		return false
	}
	if _, ok := exact[ip.String()]; ok {
		return true
	}
	for _, network := range cidrs {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func parseIPRules(rules []string) (map[string]struct{}, []*net.IPNet, error) {
	exact := make(map[string]struct{})
	cidrs := make([]*net.IPNet, 0)
	for _, raw := range rules {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if strings.Contains(raw, "/") {
			_, network, err := net.ParseCIDR(raw)
			if err != nil {
				return nil, nil, err
			}
			cidrs = append(cidrs, network)
			continue
		}
		ip := net.ParseIP(raw)
		if ip == nil {
			return nil, nil, fmt.Errorf("invalid ip %q", raw)
		}
		exact[ip.String()] = struct{}{}
	}
	return exact, cidrs, nil
}
