#!/usr/bin/env python3
"""Public release readiness checks for ARE Foundation."""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
REQUIRED_FILES = [
    "README.md",
    "LICENSE",
    "NOTICE",
    "SECURITY.md",
    "CONTRIBUTING.md",
    "CODE_OF_CONDUCT.md",
    "VERSION",
    ".github/CODEOWNERS",
    ".github/dependabot.yml",
    ".github/workflows/foundation-ci.yml",
    ".github/workflows/release-readiness.yml",
    ".github/pull_request_template.md",
    "docs/public-boundary.md",
    "docs/deployment-boundary.md",
    "docs/dev-mode-security.md",
    "docs/threat-model.md",
    "docs/oss-release-checklist.md",
    "docs/release-notes/v0.1.0.md",
    "api/openapi.yaml",
    "deploy/compose/docker-compose.yml",
    "tools/security/secret_scan.py",
]
FORBIDDEN_TRACKED_PREFIXES = [
    ".are-gates/",
    ".env",
    "reports/",
    "node_modules/",
    "target/",
    "deploy/compose/tls/",
]
REQUIRED_BOUNDARY_TEXT = [
    "does not execute customer actions",
    "does not claim certification",
    "Command Center",
    "S2-S6",
    "governance-strata internals",
    "local Docker Compose",
]
REQUIRED_OPENAPI_TEXT = [
    "operationId:",
    "requestBody:",
    "AgentCreateRequest:",
    "PassportIssueRequest:",
    "ScopeEvaluateRequest:",
    "PolicyEvaluationRequest:",
    "ErrorEnvelope:",
    "/v1/platform/health:",
    "/health:",
]


def git_files() -> list[str]:
    out = subprocess.check_output(["git", "ls-files"], cwd=ROOT, text=True)
    return [line.strip().replace("\\", "/") for line in out.splitlines() if line.strip()]


def run_secret_scan() -> int:
    return subprocess.run([sys.executable, "tools/security/secret_scan.py"], cwd=ROOT).returncode


def main() -> int:
    findings: list[str] = []
    tracked = set(git_files())
    for rel in REQUIRED_FILES:
        if not (ROOT / rel).exists():
            findings.append(f"missing required release file: {rel}")
    for rel in tracked:
        for prefix in FORBIDDEN_TRACKED_PREFIXES:
            if rel == prefix.rstrip("/") or rel.startswith(prefix):
                findings.append(f"forbidden tracked path: {rel}")
    readme = (ROOT / "README.md").read_text(encoding="utf-8")
    for text in REQUIRED_BOUNDARY_TEXT:
        if text not in readme:
            findings.append(f"README missing boundary text: {text}")
    openapi = (ROOT / "api/openapi.yaml").read_text(encoding="utf-8")
    for text in REQUIRED_OPENAPI_TEXT:
        if text not in openapi:
            findings.append(f"OpenAPI missing contract detail: {text}")
    if run_secret_scan() != 0:
        findings.append("secret scan failed")
    if findings:
        print("release audit failed:", file=sys.stderr)
        for finding in findings:
            print(f"- {finding}", file=sys.stderr)
        return 1
    print("release audit passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
