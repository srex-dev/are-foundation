#!/usr/bin/env python3
"""Run a public-safe ARE Foundation smoke flow."""

from __future__ import annotations

import json
import os
import time
import urllib.error
import urllib.request
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
REPORT_DIR = ROOT / "reports" / "foundation-smoke"
BASE = os.environ.get("ARE_FOUNDATION_BASE_URL", "http://localhost:18085").rstrip("/")
TOKEN = os.environ.get("ARE_FOUNDATION_TOKEN", "test-token")


def request(method: str, path: str, body: dict | None = None, idem: str | None = None) -> dict:
    data = None if body is None else json.dumps(body).encode("utf-8")
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("Authorization", f"Bearer {TOKEN}")
    req.add_header("X-Request-ID", f"foundation-smoke-{int(time.time() * 1000)}")
    req.add_header("X-ARE-Agent-ID", "foundation-smoke-operator")
    if data is not None:
        req.add_header("Content-Type", "application/json")
    if idem:
        req.add_header("Idempotency-Key", idem)
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            raw = resp.read().decode("utf-8")
            return {"status": resp.status, "body": json.loads(raw) if raw else {}}
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8")
        raise RuntimeError(f"{method} {path} failed status={exc.code} body={raw}") from exc


def main() -> int:
    agent = request(
        "POST",
        "/v1/identity/agents",
        {"agent_type": "demo.release-agent", "owner_id": "demo-owner", "metadata": {"public": True}},
        "smoke-agent",
    )
    agent_id = agent["body"]["agent"]["agent_id"]

    passport = request(
        "POST",
        "/v1/passports",
        {
            "agent_id": agent_id,
            "passport_type": "standard",
            "requested_scopes": [{"action_class": "model.promote_to_production", "resource_pattern": "model/*"}],
            "ttl_seconds": 3600,
            "issued_by": "demo-owner",
            "reason": "foundation smoke",
        },
        "smoke-passport",
    )
    passport_id = passport["body"]["passport"]["passport_id"]

    scope = request(
        "POST",
        "/v1/enforcement/scope:evaluate",
        {
            "agent_id": agent_id,
            "passport_id": passport_id,
            "action_class": "model.promote_to_production",
            "resource": "model/champion",
        },
        "smoke-scope",
    )
    policy = request(
        "POST",
        "/v1/policy/evaluations",
        {
            "decision_id": "smoke-policy",
            "agent_id": agent_id,
            "action_class": "model.promote_to_production",
            "resource": "model/champion",
        },
        "smoke-policy",
    )
    verify = request("POST", "/v1/passports:verify", {"agent_id": agent_id, "passport_id": passport_id}, "smoke-verify")

    summary = {
        "verdict": "GREEN",
        "base_url": BASE,
        "agent_id": agent_id,
        "passport_id": passport_id,
        "scope_effect": scope["body"]["decision"]["effect"],
        "policy_effect": policy["body"]["decision"]["effect"],
        "passport_verified": verify["body"]["verified"],
        "executed": False,
        "receipt_created": False,
        "source_refs": [
            "/v1/identity/agents",
            "/v1/passports",
            "/v1/enforcement/scope:evaluate",
            "/v1/policy/evaluations",
            "/v1/passports:verify",
        ],
    }
    REPORT_DIR.mkdir(parents=True, exist_ok=True)
    (REPORT_DIR / "summary.json").write_text(json.dumps(summary, indent=2), encoding="utf-8")
    (REPORT_DIR / "public-summary.md").write_text(
        "\n".join(
            [
                "# ARE Foundation Smoke",
                "",
                f"Verdict: **{summary['verdict']}**",
                "",
                f"- Agent: `{agent_id}`",
                f"- Passport: `{passport_id}`",
                f"- Scope decision: `{summary['scope_effect']}`",
                f"- Policy decision: `{summary['policy_effect']}`",
                f"- Passport verified: `{summary['passport_verified']}`",
                "- Executed: `false`",
                "- Receipt created: `false`",
                "",
            ]
        ),
        encoding="utf-8",
    )
    print(json.dumps(summary, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
