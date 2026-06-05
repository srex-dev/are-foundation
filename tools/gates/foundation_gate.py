#!/usr/bin/env python3
"""Run the public ARE Foundation gate."""

from __future__ import annotations

import subprocess
import sys
import os
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]


def run(cmd: list[str], cwd: Path = ROOT) -> None:
    if os.name == "nt" and cmd[0] == "npm":
        cmd = ["npm.cmd", *cmd[1:]]
    print("+", " ".join(cmd), f"(cwd={cwd})")
    subprocess.run(cmd, cwd=cwd, check=True)


def main() -> int:
    try:
        run([sys.executable, "tools/security/secret_scan.py"])
        run([sys.executable, "tools/testing/cli_wrapper_check.py"])
        run(["go", "test", "./..."], ROOT / "tools" / "testing" / "policyfixtures")
        run(["go", "test", "./..."], ROOT / "s0" / "agent-registry-service")
        run(["go", "test", "./..."], ROOT / "s0" / "passport-issuance-engine")
        run(["go", "test", "./..."], ROOT / "sx" / "s0s1-rest-bff")
        run(["go", "test", "./..."], ROOT / "sx" / "are-api-gateway")
        run(["cargo", "test"], ROOT / "s0" / "immutable-ledger")
        run(["cargo", "test"], ROOT / "s1" / "scope-evaluator-runtime")
        run(["npm", "ci"], ROOT / "s1" / "opa-integration-layer")
        run(["npm", "test", "--", "--runInBand"], ROOT / "s1" / "opa-integration-layer")
    except subprocess.CalledProcessError as exc:
        return exc.returncode
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
