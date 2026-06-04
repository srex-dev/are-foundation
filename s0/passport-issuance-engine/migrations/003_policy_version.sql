ALTER TABLE passports
  ADD COLUMN IF NOT EXISTS policy_id_at_issuance TEXT,
  ADD COLUMN IF NOT EXISTS policy_ver_at_issuance TEXT;
