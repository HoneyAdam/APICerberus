package plugin

import (
	"fmt"
	"net/http"
	"strings"
)

// BotDetectConfig configures bot detection behavior.
type BotDetectConfig struct {
	AllowList []string
	DenyList  []string
	Action    string
}

// BotDetect detects bots by user-agent patterns and blocks or flags them.
type BotDetect struct {
	allowList []string
	denyList  []string
	action    string
}

func NewBotDetect(cfg BotDetectConfig) *BotDetect {
	allow := normalizeStringList(cfg.AllowList)
	deny := normalizeStringList(cfg.DenyList)

	action := strings.ToLower(strings.TrimSpace(cfg.Action))
	if action == "" {
		action = "block"
	}
	if action != "flag" {
		action = "block"
	}

	return &BotDetect{
		allowList: allow,
		denyList:  deny,
		action:    action,
	}
}

func (b *BotDetect) Name() string  { return "bot-detect" }
func (b *BotDetect) Phase() Phase  { return PhasePreAuth }
func (b *BotDetect) Priority() int { return 3 }

func (b *BotDetect) Evaluate(in *PipelineContext) error {
	if b == nil || in == nil || in.Request == nil {
		return nil
	}
	ua := strings.ToLower(strings.TrimSpace(in.Request.Header.Get("User-Agent")))
	if ua == "" {
		return nil
	}
	if matchesAny(ua, b.allowList) {
		return nil
	}

	isBot := matchesAny(ua, b.denyList) || matchesAny(ua, knownBotPatterns)
	if !isBot {
		return nil
	}

	if in.Metadata == nil {
		in.Metadata = map[string]any{}
	}
	in.Metadata["bot_detected"] = true
	in.Metadata["bot_user_agent"] = in.Request.Header.Get("User-Agent")

	if b.action == "flag" {
		if in.ResponseWriter != nil {
			in.ResponseWriter.Header().Set("X-Bot-Detected", "true")
		}
		return nil
	}

	return &BotDetectError{
		PluginError: PluginError{
			Code:    "bot_blocked",
			Message: fmt.Sprintf("Blocked bot user-agent: %s", in.Request.Header.Get("User-Agent")),
			Status:  http.StatusForbidden,
		},
	}
}

var knownBotPatterns = []string{
	"googlebot",
	"bingbot",
	"yandexbot",
	"duckduckbot",
	"baiduspider",
	"slurp",
	"spider",
	"crawler",
	"bot/",
	"bot ",
}

func matchesAny(userAgent string, patterns []string) bool {
	if userAgent == "" || len(patterns) == 0 {
		return false
	}
	for _, pattern := range patterns {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		if strings.Contains(userAgent, pattern) {
			return true
		}
	}
	return false
}

// BotDetectError indicates blocked bot traffic.
type BotDetectError struct {
	PluginError
}
