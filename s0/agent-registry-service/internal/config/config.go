package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

// Config is runtime configuration for the registry service.
type Config struct {
	DBConnectionString    string
	KafkaBootstrapServers string
	KafkaSASLUsername     string
	KafkaSASLPassword     string
	RedisConnectionString string
	GRPCPort              int
	MetricsPort           int
	HealthPort            int
	KafkaTopicLifecycle   string
	OutboxPollIntervalMS  int
	OutboxMaxAttempts     int
}

// Load reads config from environment variables.
func Load() (Config, error) {
	grpcPort, err := getInt("ARE_GRPC_PORT", 9090)
	if err != nil {
		return Config{}, err
	}
	metricsPort, err := getInt("ARE_METRICS_PORT", 8081)
	if err != nil {
		return Config{}, err
	}
	healthPort, err := getInt("ARE_HEALTH_PORT", 8080)
	if err != nil {
		return Config{}, err
	}
	outboxPollMS, err := getInt("ARE_OUTBOX_POLL_INTERVAL_MS", 500)
	if err != nil {
		return Config{}, err
	}
	outboxMaxAttempts, err := getInt("ARE_OUTBOX_MAX_ATTEMPTS", 10)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		DBConnectionString:    os.Getenv("ARE_DB_CONNECTION_STRING"),
		KafkaBootstrapServers: os.Getenv("ARE_KAFKA_BOOTSTRAP_SERVERS"),
		KafkaSASLUsername:     os.Getenv("ARE_KAFKA_SASL_USERNAME"),
		KafkaSASLPassword:     os.Getenv("ARE_KAFKA_SASL_PASSWORD"),
		RedisConnectionString: os.Getenv("ARE_REDIS_CONNECTION_STRING"),
		GRPCPort:              grpcPort,
		MetricsPort:           metricsPort,
		HealthPort:            healthPort,
		KafkaTopicLifecycle:   getStr("ARE_KAFKA_TOPIC_LIFECYCLE", "are.agents.lifecycle"),
		OutboxPollIntervalMS:  outboxPollMS,
		OutboxMaxAttempts:     outboxMaxAttempts,
	}
	if cfg.DBConnectionString == "" {
		return Config{}, errors.New("ARE_DB_CONNECTION_STRING is required")
	}
	if cfg.KafkaBootstrapServers == "" {
		return Config{}, errors.New("ARE_KAFKA_BOOTSTRAP_SERVERS is required")
	}
	if cfg.KafkaSASLUsername == "" || cfg.KafkaSASLPassword == "" {
		return Config{}, errors.New("ARE_KAFKA_SASL_USERNAME and ARE_KAFKA_SASL_PASSWORD are required")
	}
	return cfg, nil
}

func getStr(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func getInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid int value for %s: %q", key, v)
	}
	return n, nil
}
