#!/usr/bin/env python3
"""Generate local-only mTLS certs for ARE Foundation compose."""

from __future__ import annotations

import shutil
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
TLS = ROOT / "deploy" / "compose" / "tls"


def run(args: list[str]) -> None:
    subprocess.run(args, cwd=TLS, check=True)


def main() -> int:
    TLS.mkdir(parents=True, exist_ok=True)
    ca_crt = TLS / "ca.crt"
    cert = TLS / "foundation.crt"
    key = TLS / "foundation.key"
    if ca_crt.exists() and cert.exists() and key.exists():
        print(f"dev certs already exist under {TLS}")
        return 0
    if shutil.which("openssl") is None:
        if shutil.which("go") is None:
            print("openssl or go is required to generate local dev certs", file=sys.stderr)
            return 2
        subprocess.run(["go", "run", "tools/gates/devcert_gen.go"], cwd=ROOT, check=True)
        return 0

    (TLS / "foundation.ext").write_text(
        "\n".join(
            [
                "basicConstraints=CA:FALSE",
                "keyUsage=digitalSignature,keyEncipherment",
                "extendedKeyUsage=serverAuth,clientAuth",
                "subjectAltName=DNS:localhost,DNS:are-api-gateway,DNS:s0s1-rest-bff,IP:127.0.0.1",
                "",
            ]
        ),
        encoding="utf-8",
    )
    run(["openssl", "genrsa", "-out", "ca.key", "2048"])
    run(["openssl", "req", "-x509", "-new", "-nodes", "-key", "ca.key", "-sha256", "-days", "3650", "-subj", "/CN=ARE Foundation Local CA", "-out", "ca.crt"])
    run(["openssl", "genrsa", "-out", "foundation.key", "2048"])
    run(["openssl", "req", "-new", "-key", "foundation.key", "-subj", "/CN=s0s1-rest-bff", "-out", "foundation.csr"])
    run(["openssl", "x509", "-req", "-in", "foundation.csr", "-CA", "ca.crt", "-CAkey", "ca.key", "-CAcreateserial", "-out", "foundation.crt", "-days", "825", "-sha256", "-extfile", "foundation.ext"])
    print(f"generated local dev certs under {TLS}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
