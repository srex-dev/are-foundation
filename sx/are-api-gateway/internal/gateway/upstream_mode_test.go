package gateway

import (
	"os"
	"testing"
)

func TestLoadUpstreamModeDefaultStatic(t *testing.T) {
	_ = os.Unsetenv("ARE_GW_UPSTREAM_MODE")
	m, err := LoadUpstreamMode()
	if err != nil || m != UpstreamModeStatic {
		t.Fatalf("got %v %v", m, err)
	}
}

func TestLoadUpstreamModeInvalid(t *testing.T) {
	t.Setenv("ARE_GW_UPSTREAM_MODE", "banana")
	if _, err := LoadUpstreamMode(); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateLiveRequiresS0S1Proxy(t *testing.T) {
	t.Setenv("ARE_GW_S0S1_HTTP_PROXY_BASE", "")
	if err := ValidateUpstreamModeRequirements(UpstreamModeLive); err == nil {
		t.Fatal("expected error")
	}
	t.Setenv("ARE_GW_S0S1_HTTP_PROXY_BASE", "https://bff:8443")
	if err := ValidateUpstreamModeRequirements(UpstreamModeLive); err != nil {
		t.Fatal(err)
	}
}

func TestValidateStaticSkips(t *testing.T) {
	_ = os.Unsetenv("ARE_GW_S0S1_HTTP_PROXY_BASE")
	if err := ValidateUpstreamModeRequirements(UpstreamModeStatic); err != nil {
		t.Fatal(err)
	}
}
