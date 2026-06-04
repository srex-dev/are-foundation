package config

import "testing"

func TestLoadRequiresMandatoryFields(t *testing.T) {
	t.Setenv("ARE_DB_CONNECTION_STRING", "")
	t.Setenv("ARE_KAFKA_BOOTSTRAP_SERVERS", "")
	t.Setenv("ARE_KAFKA_SASL_USERNAME", "")
	t.Setenv("ARE_KAFKA_SASL_PASSWORD", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing required environment")
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("ARE_DB_CONNECTION_STRING", "postgres://u:p@localhost:5432/are")
	t.Setenv("ARE_KAFKA_BOOTSTRAP_SERVERS", "localhost:9092")
	t.Setenv("ARE_KAFKA_SASL_USERNAME", "u")
	t.Setenv("ARE_KAFKA_SASL_PASSWORD", "p")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GRPCPort != 9090 || cfg.MetricsPort != 8081 || cfg.HealthPort != 8080 {
		t.Fatalf("unexpected default ports: %+v", cfg)
	}
}

func TestGetIntRejectsInvalidValue(t *testing.T) {
	t.Setenv("ARE_INVALID_INT", "bad")
	_, err := getInt("ARE_INVALID_INT", 1)
	if err == nil {
		t.Fatal("expected error for invalid int")
	}
}

func TestLoadRejectsInvalidIntEnv(t *testing.T) {
	t.Setenv("ARE_DB_CONNECTION_STRING", "postgres://u:p@localhost:5432/are")
	t.Setenv("ARE_KAFKA_BOOTSTRAP_SERVERS", "localhost:9092")
	t.Setenv("ARE_KAFKA_SASL_USERNAME", "u")
	t.Setenv("ARE_KAFKA_SASL_PASSWORD", "p")
	t.Setenv("ARE_GRPC_PORT", "not-a-port")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid ARE_GRPC_PORT")
	}
}

func TestLoadWithOverrides(t *testing.T) {
	t.Setenv("ARE_DB_CONNECTION_STRING", "postgres://u:p@localhost:5432/are")
	t.Setenv("ARE_KAFKA_BOOTSTRAP_SERVERS", "localhost:9092")
	t.Setenv("ARE_KAFKA_SASL_USERNAME", "u")
	t.Setenv("ARE_KAFKA_SASL_PASSWORD", "p")
	t.Setenv("ARE_GRPC_PORT", "10090")
	t.Setenv("ARE_METRICS_PORT", "10081")
	t.Setenv("ARE_HEALTH_PORT", "10080")
	t.Setenv("ARE_KAFKA_TOPIC_LIFECYCLE", "custom.lifecycle")
	t.Setenv("ARE_OUTBOX_POLL_INTERVAL_MS", "750")
	t.Setenv("ARE_OUTBOX_MAX_ATTEMPTS", "12")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GRPCPort != 10090 || cfg.MetricsPort != 10081 || cfg.HealthPort != 10080 {
		t.Fatalf("unexpected port overrides: %+v", cfg)
	}
	if cfg.KafkaTopicLifecycle != "custom.lifecycle" || cfg.OutboxPollIntervalMS != 750 || cfg.OutboxMaxAttempts != 12 {
		t.Fatalf("unexpected override values: %+v", cfg)
	}
}
