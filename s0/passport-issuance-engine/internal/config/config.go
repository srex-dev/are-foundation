package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds process listen configuration.
type Config struct {
	GRPCPort    int
	HealthPort  int
	MetricsPort int
}

// Load reads configuration from environment (defaults match k8s deployment).
func Load() (Config, error) {
	c := Config{
		GRPCPort:    getenvInt("ARE_PASSPORT_GRPC_PORT", 9094),
		HealthPort:  getenvInt("ARE_PASSPORT_HEALTH_PORT", 8080),
		MetricsPort: getenvInt("ARE_PASSPORT_METRICS_PORT", 8085),
	}
	if c.GRPCPort <= 0 || c.HealthPort <= 0 || c.MetricsPort <= 0 {
		return Config{}, fmt.Errorf("invalid listen ports")
	}
	return c, nil
}

func getenvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
