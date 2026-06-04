#!/usr/bin/env bash
set -euo pipefail

CONTAINER_NAME="${CONTAINER_NAME:-are-ledger-perm-check}"
POSTGRES_IMAGE="${POSTGRES_IMAGE:-postgres:15-alpine}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-postgres}"
POSTGRES_PORT="${POSTGRES_PORT:-55432}"

cleanup() {
  docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
docker run -d --name "${CONTAINER_NAME}" \
  -e POSTGRES_PASSWORD="${POSTGRES_PASSWORD}" \
  -p "${POSTGRES_PORT}:5432" \
  "${POSTGRES_IMAGE}" >/dev/null

until docker exec "${CONTAINER_NAME}" pg_isready -U postgres >/dev/null 2>&1; do
  sleep 1
done

docker exec -i "${CONTAINER_NAME}" psql -U postgres <<'SQL'
CREATE SCHEMA IF NOT EXISTS are_ledger;
CREATE TABLE IF NOT EXISTS are_ledger.ledger_entries (
  entry_id UUID PRIMARY KEY,
  content BYTEA NOT NULL
);
CREATE ROLE are_ledger_writer LOGIN PASSWORD 'writerpass';
GRANT USAGE ON SCHEMA are_ledger TO are_ledger_writer;
GRANT SELECT, INSERT ON are_ledger.ledger_entries TO are_ledger_writer;
REVOKE UPDATE, DELETE ON are_ledger.ledger_entries FROM are_ledger_writer;
SQL

set +e
docker exec -e PGPASSWORD=writerpass -i "${CONTAINER_NAME}" \
  psql -U are_ledger_writer -d postgres -v ON_ERROR_STOP=1 \
  -c "UPDATE are_ledger.ledger_entries SET content = E'\\\\x00'::bytea;" >/dev/null 2>&1
UPDATE_EXIT=$?
docker exec -e PGPASSWORD=writerpass -i "${CONTAINER_NAME}" \
  psql -U are_ledger_writer -d postgres -v ON_ERROR_STOP=1 \
  -c "DELETE FROM are_ledger.ledger_entries;" >/dev/null 2>&1
DELETE_EXIT=$?
set -e

if [[ "${UPDATE_EXIT}" -eq 0 || "${DELETE_EXIT}" -eq 0 ]]; then
  echo "FAIL: expected UPDATE/DELETE to be denied for are_ledger_writer"
  exit 1
fi

echo "PASS: UPDATE/DELETE denied for are_ledger_writer on are_ledger.ledger_entries"

