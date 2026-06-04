package main

import (
	"testing"

	"github.com/srex-dev/are-foundation/s0/passport-issuance-engine/internal/config"
)

func TestConfigLoadDefaults(t *testing.T) {
	t.Setenv("ARE_PASSPORT_GRPC_PORT", "")
	t.Setenv("ARE_PASSPORT_HEALTH_PORT", "")
	t.Setenv("ARE_PASSPORT_METRICS_PORT", "")
	c, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.GRPCPort != 9094 || c.HealthPort != 8080 || c.MetricsPort != 8085 {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}
