#!/usr/bin/env python3
"""Sanity-check the developer CLI and Homebrew packaging template."""

from __future__ import annotations

import shutil
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
CLI = ROOT / "bin" / "are-foundation"
FORMULA_TEMPLATE = ROOT / "packaging" / "homebrew" / "Formula" / "are-foundation.rb.template"
HOMEBREW_DOC = ROOT / "docs" / "homebrew.md"


def fail(message: str) -> int:
    print(f"cli wrapper check failed: {message}", file=sys.stderr)
    return 1


def main() -> int:
    if not CLI.is_file():
        return fail("bin/are-foundation is missing")
    cli_text = CLI.read_text(encoding="utf-8")
    if not cli_text.startswith("#!/usr/bin/env sh"):
        return fail("CLI must use a portable sh shebang")
    for expected in ("up)", "smoke)", "pressure)", "gate)", "release-audit)"):
        if expected not in cli_text:
            return fail(f"CLI missing command case {expected}")

    if not FORMULA_TEMPLATE.is_file():
        return fail("Homebrew formula template is missing")
    formula = FORMULA_TEMPLATE.read_text(encoding="utf-8")
    for expected in (
        "class AreFoundation < Formula",
        'license "Apache-2.0"',
        'depends_on "docker"',
        "__VERSION__",
        "__SHA256__",
    ):
        if expected not in formula:
            return fail(f"formula template missing {expected}")

    if not HOMEBREW_DOC.is_file():
        return fail("docs/homebrew.md is missing")
    doc = HOMEBREW_DOC.read_text(encoding="utf-8")
    for expected in ("brew tap srex-dev/are", "Docker Compose", "runtime"):
        if expected not in doc:
            return fail(f"Homebrew docs missing {expected}")

    sh = shutil.which("sh")
    if sh:
        help_output = subprocess.check_output([sh, str(CLI), "help"], cwd=ROOT, text=True)
        version_output = subprocess.check_output([sh, str(CLI), "version"], cwd=ROOT, text=True)
        if "Docker Compose remains the runtime" not in help_output:
            return fail("CLI help does not explain Docker Compose runtime boundary")
        if "ARE Foundation" not in version_output:
            return fail("CLI version output missing product name")

    print("cli wrapper check passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
