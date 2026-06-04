use sha2::{Digest, Sha256};

pub fn sha256_hex(value: &[u8]) -> String {
    let mut hasher = Sha256::new();
    hasher.update(value);
    hex::encode(hasher.finalize())
}

pub fn canonical_entry_hash(
    entry_type: &str,
    agent_id: &str,
    content: &[u8],
    content_type: &str,
    source_id: &str,
    written_ts_ms: i64,
    previous_hash: &str,
) -> String {
    let mut bytes = Vec::with_capacity(
        entry_type.len()
            + agent_id.len()
            + content.len()
            + content_type.len()
            + source_id.len()
            + previous_hash.len()
            + 32,
    );
    bytes.extend_from_slice(entry_type.as_bytes());
    bytes.extend_from_slice(agent_id.as_bytes());
    bytes.extend_from_slice(content);
    bytes.extend_from_slice(content_type.as_bytes());
    bytes.extend_from_slice(source_id.as_bytes());
    bytes.extend_from_slice(written_ts_ms.to_string().as_bytes());
    bytes.extend_from_slice(previous_hash.as_bytes());
    sha256_hex(&bytes)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn canonical_hash_is_deterministic_for_identical_input() {
        let first = canonical_entry_hash(
            "LEDGER_ENTRY_TYPE_ACTION_RECEIPT",
            "agent-1",
            b"{\"ok\":true}",
            "application/json",
            "ARE-FOUNDATION-PROOF",
            1_700_000_000_000,
            "prev-hash",
        );
        for _ in 0..9 {
            let next = canonical_entry_hash(
                "LEDGER_ENTRY_TYPE_ACTION_RECEIPT",
                "agent-1",
                b"{\"ok\":true}",
                "application/json",
                "ARE-FOUNDATION-PROOF",
                1_700_000_000_000,
                "prev-hash",
            );
            assert_eq!(first, next);
        }
    }

    #[test]
    fn canonical_hash_changes_when_timestamp_or_previous_hash_changes() {
        let baseline = canonical_entry_hash(
            "LEDGER_ENTRY_TYPE_ACTION_RECEIPT",
            "agent-1",
            b"{\"ok\":true}",
            "application/json",
            "ARE-FOUNDATION-PROOF",
            1_700_000_000_000,
            "prev-hash",
        );
        let changed_ts = canonical_entry_hash(
            "LEDGER_ENTRY_TYPE_ACTION_RECEIPT",
            "agent-1",
            b"{\"ok\":true}",
            "application/json",
            "ARE-FOUNDATION-PROOF",
            1_700_000_000_001,
            "prev-hash",
        );
        let changed_prev = canonical_entry_hash(
            "LEDGER_ENTRY_TYPE_ACTION_RECEIPT",
            "agent-1",
            b"{\"ok\":true}",
            "application/json",
            "ARE-FOUNDATION-PROOF",
            1_700_000_000_000,
            "prev-hash-2",
        );
        assert_ne!(baseline, changed_ts);
        assert_ne!(baseline, changed_prev);
    }
}
