#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
docker run --rm -i --network host grafana/k6 run - < "${ROOT_DIR}/perf/k6-baseline.js"
