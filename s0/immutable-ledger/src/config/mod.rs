use std::env;

use thiserror::Error;

#[derive(Debug, Clone)]
pub struct AppConfig {
    pub grpc_port: u16,
    pub health_port: u16,
    pub metrics_port: u16,
    pub max_content_size_bytes: usize,
    pub db_connection_string: String,
    pub read_replica_connection_string: Option<String>,
    pub kafka_bootstrap_servers: String,
    pub kafka_sasl_username: String,
    pub kafka_sasl_password: String,
    pub genesis_hash_input: String,
}

#[derive(Debug, Error)]
pub enum ConfigError {
    #[error("missing required environment variable: {0}")]
    Missing(String),
    #[error("invalid integer value for {0}")]
    InvalidInteger(String),
}

impl AppConfig {
    pub fn from_env() -> Result<Self, ConfigError> {
        Ok(Self {
            grpc_port: parse_u16("ARE_LEDGER_GRPC_PORT", 9092)?,
            health_port: parse_u16("ARE_LEDGER_HEALTH_PORT", 8080)?,
            metrics_port: parse_u16("ARE_LEDGER_METRICS_PORT", 8083)?,
            max_content_size_bytes: parse_usize("ARE_LEDGER_MAX_CONTENT_SIZE_BYTES", 1_048_576)?,
            db_connection_string: required("ARE_LEDGER_DB_CONNECTION_STRING")?,
            read_replica_connection_string: env::var("ARE_LEDGER_READ_REPLICA_CONNECTION_STRING")
                .ok(),
            kafka_bootstrap_servers: required("ARE_LEDGER_KAFKA_BOOTSTRAP_SERVERS")?,
            kafka_sasl_username: required("ARE_LEDGER_KAFKA_SASL_USERNAME")?,
            kafka_sasl_password: required("ARE_LEDGER_KAFKA_SASL_PASSWORD")?,
            genesis_hash_input: env::var("ARE_LEDGER_GENESIS_HASH_INPUT")
                .unwrap_or_else(|_| "ARE_LEDGER_GENESIS".to_string()),
        })
    }
}

fn required(name: &str) -> Result<String, ConfigError> {
    env::var(name).map_err(|_| ConfigError::Missing(name.to_string()))
}

fn parse_u16(name: &str, default: u16) -> Result<u16, ConfigError> {
    match env::var(name) {
        Ok(raw) => raw
            .parse::<u16>()
            .map_err(|_| ConfigError::InvalidInteger(name.to_string())),
        Err(_) => Ok(default),
    }
}

fn parse_usize(name: &str, default: usize) -> Result<usize, ConfigError> {
    match env::var(name) {
        Ok(raw) => raw
            .parse::<usize>()
            .map_err(|_| ConfigError::InvalidInteger(name.to_string())),
        Err(_) => Ok(default),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::{Mutex, OnceLock};

    fn env_lock() -> std::sync::MutexGuard<'static, ()> {
        static LOCK: OnceLock<Mutex<()>> = OnceLock::new();
        LOCK.get_or_init(|| Mutex::new(())).lock().expect("lock")
    }

    fn clear() {
        let names = [
            "ARE_LEDGER_GRPC_PORT",
            "ARE_LEDGER_HEALTH_PORT",
            "ARE_LEDGER_METRICS_PORT",
            "ARE_LEDGER_MAX_CONTENT_SIZE_BYTES",
            "ARE_LEDGER_DB_CONNECTION_STRING",
            "ARE_LEDGER_READ_REPLICA_CONNECTION_STRING",
            "ARE_LEDGER_KAFKA_BOOTSTRAP_SERVERS",
            "ARE_LEDGER_KAFKA_SASL_USERNAME",
            "ARE_LEDGER_KAFKA_SASL_PASSWORD",
            "ARE_LEDGER_GENESIS_HASH_INPUT",
        ];
        for name in names {
            std::env::remove_var(name);
        }
    }

    #[test]
    fn loads_defaults_and_required_values() {
        let _guard = env_lock();
        clear();
        std::env::set_var("ARE_LEDGER_DB_CONNECTION_STRING", "postgres://db");
        std::env::set_var("ARE_LEDGER_KAFKA_BOOTSTRAP_SERVERS", "kafka:9092");
        std::env::set_var("ARE_LEDGER_KAFKA_SASL_USERNAME", "user");
        std::env::set_var("ARE_LEDGER_KAFKA_SASL_PASSWORD", "pass");
        let cfg = AppConfig::from_env().expect("config");
        assert_eq!(cfg.grpc_port, 9092);
        assert_eq!(cfg.health_port, 8080);
        assert_eq!(cfg.metrics_port, 8083);
        assert_eq!(cfg.max_content_size_bytes, 1_048_576);
        assert_eq!(cfg.genesis_hash_input, "ARE_LEDGER_GENESIS");
    }

    #[test]
    fn missing_required_variable_fails() {
        let _guard = env_lock();
        clear();
        std::env::set_var("ARE_LEDGER_KAFKA_BOOTSTRAP_SERVERS", "kafka:9092");
        std::env::set_var("ARE_LEDGER_KAFKA_SASL_USERNAME", "user");
        std::env::set_var("ARE_LEDGER_KAFKA_SASL_PASSWORD", "pass");
        let err = AppConfig::from_env().expect_err("must fail");
        assert!(matches!(err, ConfigError::Missing(_)));
    }

    #[test]
    fn invalid_integer_fails() {
        let _guard = env_lock();
        clear();
        std::env::set_var("ARE_LEDGER_DB_CONNECTION_STRING", "postgres://db");
        std::env::set_var("ARE_LEDGER_KAFKA_BOOTSTRAP_SERVERS", "kafka:9092");
        std::env::set_var("ARE_LEDGER_KAFKA_SASL_USERNAME", "user");
        std::env::set_var("ARE_LEDGER_KAFKA_SASL_PASSWORD", "pass");
        std::env::set_var("ARE_LEDGER_GRPC_PORT", "not-a-number");
        let err = AppConfig::from_env().expect_err("must fail");
        assert!(matches!(err, ConfigError::InvalidInteger(_)));
    }
}
