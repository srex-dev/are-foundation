#!/usr/bin/env python3
"""Run a public-safe RPS ladder against ARE Foundation."""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
import time
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
PRESSURE = ROOT / "tools" / "smoke" / "foundation_pressure.py"
REPORT_DIR = ROOT / "reports" / "foundation-pressure-matrix"


def parse_levels(raw: str) -> list[float]:
    levels: list[float] = []
    for part in raw.split(","):
        value = part.strip()
        if not value:
            continue
        levels.append(float(value))
    if not levels:
        raise ValueError("at least one RPS level is required")
    return levels


def default_concurrency(target_rps: float) -> int:
    return max(8, min(128, int(target_rps / 4)))


def default_authority_pool(target_rps: float) -> int:
    return max(8, min(128, int(target_rps / 4)))


def run_level(args: argparse.Namespace, target_rps: float) -> dict:
    concurrency = args.concurrency or default_concurrency(target_rps)
    pool_size = args.authority_pool_size or default_authority_pool(target_rps)
    cmd = [
        sys.executable,
        str(PRESSURE),
        "--base-url",
        args.base_url,
        "--token",
        args.token,
        "--target-rps",
        str(target_rps),
        "--duration-seconds",
        str(args.duration_seconds),
        "--concurrency",
        str(concurrency),
        "--authority-pool-size",
        str(pool_size),
        "--max-error-rate",
        str(args.max_error_rate),
    ]
    print("+", " ".join(cmd))
    proc = subprocess.run(cmd, cwd=ROOT, text=True, capture_output=True)
    if proc.stdout:
        print(proc.stdout)
    if proc.stderr:
        print(proc.stderr, file=sys.stderr)
    try:
        summary = json.loads(proc.stdout)
    except json.JSONDecodeError:
        summary = {
            "verdict": "RED",
            "target_rps": target_rps,
            "error": "pressure runner did not return JSON",
            "stdout": proc.stdout[-2000:],
            "stderr": proc.stderr[-2000:],
        }
    summary["process_exit_code"] = proc.returncode
    summary["pressure_matrix_concurrency"] = concurrency
    summary["pressure_matrix_authority_pool_size"] = pool_size
    return summary


def matrix_verdict(rows: list[dict], p99_watch_ms: float) -> str:
    if any(row.get("verdict") == "RED" or row.get("process_exit_code") != 0 for row in rows):
        return "RED"
    if any(float(row.get("latency", {}).get("p99_ms", 0.0)) > p99_watch_ms for row in rows):
        return "WATCH"
    return "GREEN"


def write_reports(run_id: str, payload: dict) -> None:
    run_dir = REPORT_DIR / run_id
    run_dir.mkdir(parents=True, exist_ok=True)
    (run_dir / "matrix.json").write_text(json.dumps(payload, indent=2), encoding="utf-8")
    (REPORT_DIR / "latest-matrix.json").write_text(json.dumps(payload, indent=2), encoding="utf-8")
    lines = [
        "# ARE Foundation Pressure Matrix",
        "",
        f"Verdict: **{payload['verdict']}**",
        "",
        "| Target RPS | Achieved RPS | Requests | Errors | p95 ms | p99 ms | Verdict |",
        "|---:|---:|---:|---:|---:|---:|---|",
    ]
    for row in payload["rows"]:
        lines.append(
            "| {target} | {achieved} | {requests} | {errors} | {p95} | {p99} | {verdict} |".format(
                target=row.get("target_rps"),
                achieved=row.get("achieved_rps"),
                requests=row.get("total_requests"),
                errors=row.get("error_count"),
                p95=row.get("latency", {}).get("p95_ms"),
                p99=row.get("latency", {}).get("p99_ms"),
                verdict=row.get("verdict"),
            )
        )
    lines.extend(
        [
            "",
            "- Executed: `false`",
            "- Receipt created: `false`",
            "- Scope: OSS S0/S1 authority, scope, policy, and passport checks only.",
            "",
        ]
    )
    (run_dir / "public-summary.md").write_text("\n".join(lines), encoding="utf-8")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run an ARE Foundation pressure-test RPS ladder.")
    parser.add_argument("--base-url", default="http://localhost:18085")
    parser.add_argument("--token", default="test-token")
    parser.add_argument("--levels", default="50,100,200,400")
    parser.add_argument("--duration-seconds", type=float, default=30.0)
    parser.add_argument("--concurrency", type=int, default=0, help="Override concurrency for every level. Default scales with RPS.")
    parser.add_argument("--authority-pool-size", type=int, default=0, help="Override authority pool size for every level. Default scales with RPS.")
    parser.add_argument("--max-error-rate", type=float, default=0.0)
    parser.add_argument("--p99-watch-ms", type=float, default=250.0)
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    run_id = time.strftime("%Y%m%dT%H%M%SZ", time.gmtime())
    rows = [run_level(args, level) for level in parse_levels(args.levels)]
    payload = {
        "run_id": run_id,
        "verdict": matrix_verdict(rows, args.p99_watch_ms),
        "p99_watch_ms": args.p99_watch_ms,
        "duration_seconds_per_level": args.duration_seconds,
        "rows": rows,
        "executed": False,
        "receipt_created": False,
    }
    write_reports(run_id, payload)
    print(json.dumps(payload, indent=2))
    return 1 if payload["verdict"] == "RED" else 0


if __name__ == "__main__":
    raise SystemExit(main())
