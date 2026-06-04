use lazy_static::lazy_static;
use prometheus::{register_counter_vec, register_histogram_vec, CounterVec, Encoder, HistogramVec, TextEncoder};

lazy_static! {
    /// Scope evaluations: `result` is `permit` or `deny`.
    pub static ref SCOPE_EVAL_TOTAL: CounterVec = register_counter_vec!(
        "are_scope_eval_total",
        "Scope evaluations by outcome",
        &["result"]
    )
    .expect("register are_scope_eval_total");
    pub static ref SCOPE_EVAL_DURATION_SECONDS: HistogramVec = register_histogram_vec!(
        "are_scope_eval_duration_seconds",
        "Scope evaluation wall time",
        &["result"],
        vec![0.000_5, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5]
    )
    .expect("register are_scope_eval_duration_seconds");
}

pub fn encode_prometheus() -> Vec<u8> {
    let mut buf = Vec::new();
    let enc = TextEncoder::new();
    let mf = prometheus::gather();
    let _ = enc.encode(&mf, &mut buf);
    buf
}
