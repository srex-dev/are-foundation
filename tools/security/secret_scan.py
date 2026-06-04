#!/usr/bin/env python3
"""Small export hygiene scan for obvious secret/private proof material."""

from __future__ import annotations

import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SKIP_DIRS = {".git", "node_modules", "target", "dist", "reports", "deploy/compose/tls"}
ALLOW_PRIVATE_MARKER_FILES = {
    ".gitignore",
    "README.md",
    "NOTICE",
    "SECURITY.md",
    "CONTRIBUTING.md",
    "docs/public-boundary.md",
    "docs/governance-strata-integration.md",
    "tools/security/secret_scan.py",
}
PATTERNS = [
    re.compile(r"cfat_[A-Za-z0-9_-]{20,}"),
    re.compile(r"sk_[A-Za-z0-9]{20,}"),
    re.compile(r"-----BEGIN (RSA |EC |OPENSSH |)PRIVATE KEY-----"),
    re.compile(r"Authorization:\s*Bearer\s+[A-Za-z0-9._-]{20,}", re.I),
    re.compile(r"(?i)(api[_-]?key|secret|credential|password)\s*[:=]\s*['\"][^'\"]{12,}['\"]"),
]
PRIVATE_MARKERS = [
    ".are-gates",
    "command-center-bff",
    "visual rag",
    "client demo frontend",
]


def should_skip(path: Path) -> bool:
	rel = path.relative_to(ROOT).as_posix()
	if any(part in {"node_modules", "target", "dist", ".git"} for part in path.relative_to(ROOT).parts):
		return True
	return any(rel == d or rel.startswith(d + "/") for d in SKIP_DIRS)


def main() -> int:
    findings: list[str] = []
    for path in ROOT.rglob("*"):
        if not path.is_file() or should_skip(path):
            continue
        if path.suffix.lower() in {".png", ".jpg", ".jpeg", ".gif", ".ico", ".exe", ".dll"}:
            continue
        try:
            text = path.read_text(encoding="utf-8")
        except UnicodeDecodeError:
            continue
        rel = path.relative_to(ROOT).as_posix()
        if rel != "tools/security/secret_scan.py":
            for pattern in PATTERNS:
                for match in pattern.finditer(text):
                    if "${" in match.group(0):
                        continue
                    findings.append(f"{rel}: matched {pattern.pattern}")
        lowered = text.lower()
        for marker in PRIVATE_MARKERS:
            if marker in lowered and rel not in ALLOW_PRIVATE_MARKER_FILES:
                findings.append(f"{rel}: private/commercial marker {marker!r}")
    if findings:
        print("secret/private export scan failed:", file=sys.stderr)
        for finding in findings:
            print(f"- {finding}", file=sys.stderr)
        return 1
    print("secret/private export scan passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
