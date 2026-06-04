#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
if [ -z "${KUBECONFORM_BIN:-}" ]; then
  KUBECONFORM_BIN="$(command -v kubeconform 2>/dev/null || command -v kubeconform.exe 2>/dev/null || true)"
fi

if [ -z "${KUBECONFORM_BIN}" ] || [ ! -x "${KUBECONFORM_BIN}" ]; then
  echo "kubeconform not found. Install with: go install github.com/yannh/kubeconform/cmd/kubeconform@latest"
  echo "Or set KUBECONFORM_BIN to the executable."
  exit 1
fi

"${KUBECONFORM_BIN}" -strict -summary "${ROOT_DIR}/k8s/"*.yaml
echo "k8s manifests valid"
