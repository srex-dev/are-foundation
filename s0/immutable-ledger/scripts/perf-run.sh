#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
K6_IMAGE="${K6_IMAGE:-grafana/k6:latest}"
K6_TARGET="${K6_TARGET:-host.docker.internal:9092}"

export MSYS_NO_PATHCONV=1
export MSYS2_ARG_CONV_EXCL="*"

docker run --rm \
  -e "K6_TARGET=${K6_TARGET}" \
  -v "${PROJECT_ROOT}:/work" \
  "${K6_IMAGE}" run "/work/scripts/perf/k6-baseline.js"

