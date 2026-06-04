#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
IMAGE_TAG="${1:-are/agent-registry-service:local}"
if [ -z "${TRIVY_BIN:-}" ]; then
  TRIVY_BIN="$(command -v trivy 2>/dev/null || command -v trivy.exe 2>/dev/null || true)"
fi
OUT_DIR="${ROOT_DIR}/.artifacts/security"
mkdir -p "${OUT_DIR}"

if [ -z "${TRIVY_BIN}" ] || [ ! -x "${TRIVY_BIN}" ]; then
  echo "trivy not found. Install Trivy and ensure it is on PATH, or set TRIVY_BIN to the executable."
  exit 1
fi

docker build -t "${IMAGE_TAG}" -f "${ROOT_DIR}/Dockerfile" "${ROOT_DIR}"
"${TRIVY_BIN}" image --severity HIGH,CRITICAL --exit-code 1 --format json --output "${OUT_DIR}/trivy-image.json" "${IMAGE_TAG}"
echo "trivy report: ${OUT_DIR}/trivy-image.json"
