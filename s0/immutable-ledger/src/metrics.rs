//! Prometheus metrics for the immutable ledger service.

use lazy_static::lazy_static;
use prometheus::{
    register_counter, register_counter_vec, Counter, CounterVec, Encoder, TextEncoder,
};

lazy_static! {
    pub static ref LEDGER_WRITE_TOTAL: CounterVec = register_counter_vec!(
        "are_ledger_write_total",
        "Ledger write attempts by outcome",
        &["result"]
    )
    .expect("register are_ledger_write_total");
    pub static ref LEDGER_CHAIN_VERIFY_FAILURE_TOTAL: Counter = register_counter!(
        "are_ledger_chain_verify_failure_total",
        "Chain verification detected invalid link or hash"
    )
    .expect("register are_ledger_chain_verify_failure_total");
    pub static ref OUTBOX_PUBLISH_FAILURE_TOTAL: Counter = register_counter!(
        "are_outbox_publish_failure_total",
        "Outbox Kafka publish failures (record stays pending)"
    )
    .expect("register are_outbox_publish_failure_total");
}

pub fn inc_write(result: &'static str) {
    LEDGER_WRITE_TOTAL.with_label_values(&[result]).inc();
}

pub fn encode_prometheus() -> Vec<u8> {
    let enc = TextEncoder::new();
    let mut buf = Vec::new();
    let _ = enc.encode(&prometheus::gather(), &mut buf);
    buf
}
