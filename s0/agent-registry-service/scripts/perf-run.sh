#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="${ROOT_DIR}/.artifacts/perf"
mkdir -p "${OUT_DIR}"

if ! command -v k6 >/dev/null 2>&1; then
  echo "k6 is required for IG-006 performance baseline."
  echo "Install k6 and rerun: scripts/perf-run.sh"
  exit 1
fi

k6 run "${ROOT_DIR}/scripts/perf/k6-baseline.js" --summary-export "${OUT_DIR}/k6-summary.json"
echo "performance summary: ${OUT_DIR}/k6-summary.json"
