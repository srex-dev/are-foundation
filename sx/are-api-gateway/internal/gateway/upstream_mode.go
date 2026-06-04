package gateway

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// UpstreamMode selects how REST upstream routing is satisfied.
type UpstreamMode string

const (
	// UpstreamModeStatic allows demo StaticUpstreamClient without requiring live HTTP proxy targets.
	UpstreamModeStatic UpstreamMode = "static"
	// UpstreamModeMixed allows static plus optional live HTTP proxy for S0/S1 routes when configured.
	UpstreamModeMixed UpstreamMode = "mixed"
	// UpstreamModeLive requires live upstream targets (fail-closed when unset).
	UpstreamModeLive UpstreamMode = "live"
)

// LoadUpstreamMode reads ARE_GW_UPSTREAM_MODE (default static for backward compatibility).
func LoadUpstreamMode() (UpstreamMode, error) {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("ARE_GW_UPSTREAM_MODE")))
	if v == "" {
		return UpstreamModeStatic, nil
	}
	switch v {
	case "static":
		return UpstreamModeStatic, nil
	case "mixed":
		return UpstreamModeMixed, nil
	case "live":
		return UpstreamModeLive, nil
	default:
		return "", fmt.Errorf("ARE_GW_UPSTREAM_MODE must be static, mixed, or live, got %q", v)
	}
}

// ValidateUpstreamModeRequirements enforces live upstream env when mode is live.
func ValidateUpstreamModeRequirements(mode UpstreamMode) error {
	if mode != UpstreamModeLive {
		return nil
	}
	if strings.TrimSpace(os.Getenv("ARE_GW_S0S1_HTTP_PROXY_BASE")) == "" {
		return errors.New("ARE_GW_UPSTREAM_MODE=live requires non-empty ARE_GW_S0S1_HTTP_PROXY_BASE")
	}
	return nil
}
