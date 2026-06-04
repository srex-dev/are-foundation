-- correlation_id is an opaque string (not necessarily a UUID)
ALTER TABLE are_ledger.ledger_entries
  ALTER COLUMN correlation_id TYPE VARCHAR(255) USING (
    CASE WHEN correlation_id IS NULL THEN NULL ELSE correlation_id::text END
  );
