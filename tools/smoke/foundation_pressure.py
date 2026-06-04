#!/usr/bin/env python3
"""Run a public-safe pressure test against ARE Foundation.

The test exercises the OSS S0/S1 authority path only:

setup: register fake agents -> issue scoped passports
load: verify passport -> evaluate scope -> evaluate policy -> list passports

It never calls an execution endpoint, never creates a customer-action receipt, and
only writes public-safe aggregate metrics under reports/foundation-pressure/.
"""

from __future__ import annotations

import argparse
import json
import math
import os
import queue
import statistics
import threading
import time
import urllib.error
import urllib.request
from collections import Counter, defaultdict
from dataclasses import dataclass
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
REPORT_DIR = ROOT / "reports" / "foundation-pressure"
DEFAULT_BASE = os.environ.get("ARE_FOUNDATION_BASE_URL", "http://localhost:18085").rstrip("/")
DEFAULT_TOKEN = os.environ.get("ARE_FOUNDATION_TOKEN", "test-token")
ACTION_CLASS = "model.promote_to_production"
RESOURCE = "model/champion"


@dataclass(frozen=True)
class AuthorityPair:
    agent_id: str
    passport_id: str


@dataclass
class Sample:
    name: str
    ok: bool
    latency_ms: float
    status: int | None = None
    error: str | None = None


class RateLimiter:
    def __init__(self, target_rps: float) -> None:
        self.target_rps = target_rps
        self.next_at = time.perf_counter()
        self.lock = threading.Lock()

    def wait(self) -> None:
        if self.target_rps <= 0:
            return
        with self.lock:
            now = time.perf_counter()
            scheduled = max(now, self.next_at)
            self.next_at = scheduled + (1.0 / self.target_rps)
        delay = scheduled - now
        if delay > 0:
            time.sleep(delay)


def percentile(values: list[float], pct: float) -> float:
    if not values:
        return 0.0
    ordered = sorted(values)
    index = max(0, min(len(ordered) - 1, math.ceil((pct / 100.0) * len(ordered)) - 1))
    return ordered[index]


def request(
    base_url: str,
    token: str,
    method: str,
    path: str,
    body: dict | None = None,
    idem: str | None = None,
    request_id: str | None = None,
) -> tuple[int, dict]:
    data = None if body is None else json.dumps(body).encode("utf-8")
    req = urllib.request.Request(base_url + path, data=data, method=method)
    req.add_header("Authorization", f"Bearer {token}")
    req.add_header("X-Request-ID", request_id or f"foundation-pressure-{int(time.time() * 1000)}")
    req.add_header("X-ARE-Agent-ID", "foundation-pressure-operator")
    if data is not None:
        req.add_header("Content-Type", "application/json")
    if idem:
        req.add_header("Idempotency-Key", idem)
    with urllib.request.urlopen(req, timeout=20) as resp:
        raw = resp.read().decode("utf-8")
        return resp.status, json.loads(raw) if raw else {}


def setup_authority_pool(base_url: str, token: str, pool_size: int, run_id: str) -> list[AuthorityPair]:
    pairs: list[AuthorityPair] = []
    for index in range(pool_size):
        suffix = f"{run_id}-{index}"
        _, agent = request(
            base_url,
            token,
            "POST",
            "/v1/identity/agents",
            {
                "agent_type": "demo.release-agent",
                "owner_id": "demo-owner",
                "metadata": {"public": True, "pressure": True},
            },
            idem=f"pressure-agent-{suffix}",
            request_id=f"foundation-pressure-agent-{suffix}",
        )
        agent_id = agent["agent"]["agent_id"]
        _, passport = request(
            base_url,
            token,
            "POST",
            "/v1/passports",
            {
                "agent_id": agent_id,
                "passport_type": "standard",
                "requested_scopes": [{"action_class": ACTION_CLASS, "resource_pattern": "model/*"}],
                "ttl_seconds": 3600,
                "issued_by": "demo-owner",
                "reason": "foundation pressure test",
            },
            idem=f"pressure-passport-{suffix}",
            request_id=f"foundation-pressure-passport-{suffix}",
        )
        pairs.append(AuthorityPair(agent_id=agent_id, passport_id=passport["passport"]["passport_id"]))
    return pairs


def run_check(base_url: str, token: str, pair: AuthorityPair, name: str, sequence: int) -> Sample:
    started = time.perf_counter()
    request_id = f"foundation-pressure-{name}-{sequence}"
    try:
        if name == "verify":
            status, body = request(
                base_url,
                token,
                "POST",
                "/v1/passports:verify",
                {"agent_id": pair.agent_id, "passport_id": pair.passport_id},
                idem=request_id,
                request_id=request_id,
            )
            ok = status == 200 and body.get("verified") is True
        elif name == "scope":
            status, body = request(
                base_url,
                token,
                "POST",
                "/v1/enforcement/scope:evaluate",
                {
                    "agent_id": pair.agent_id,
                    "passport_id": pair.passport_id,
                    "action_class": ACTION_CLASS,
                    "resource": RESOURCE,
                },
                idem=request_id,
                request_id=request_id,
            )
            ok = status == 200 and body.get("decision", {}).get("effect") in {"ALLOW", "DENY"}
        elif name == "policy":
            status, body = request(
                base_url,
                token,
                "POST",
                "/v1/policy/evaluations",
                {
                    "decision_id": request_id,
                    "agent_id": pair.agent_id,
                    "action_class": ACTION_CLASS,
                    "resource": RESOURCE,
                },
                idem=request_id,
                request_id=request_id,
            )
            ok = status == 200 and body.get("decision", {}).get("effect") in {"ALLOW", "DENY"}
        elif name == "list_passports":
            status, body = request(
                base_url,
                token,
                "GET",
                f"/v1/passports/by-agent/{pair.agent_id}",
                request_id=request_id,
            )
            ok = status == 200 and len(body.get("passports", [])) >= 1
        else:
            raise ValueError(f"unknown check name: {name}")
        return Sample(name=name, ok=ok, latency_ms=(time.perf_counter() - started) * 1000.0, status=status)
    except urllib.error.HTTPError as exc:
        return Sample(
            name=name,
            ok=False,
            latency_ms=(time.perf_counter() - started) * 1000.0,
            status=exc.code,
            error=f"http_{exc.code}",
        )
    except Exception as exc:  # noqa: BLE001 - safe aggregate error label for CLI proof.
        return Sample(name=name, ok=False, latency_ms=(time.perf_counter() - started) * 1000.0, error=type(exc).__name__)


def worker(
    base_url: str,
    token: str,
    pairs: list[AuthorityPair],
    checks: list[str],
    limiter: RateLimiter,
    stop_at: float,
    sample_queue: queue.Queue[Sample],
    sequence_lock: threading.Lock,
    sequence: list[int],
) -> None:
    local_index = 0
    while time.perf_counter() < stop_at:
        limiter.wait()
        with sequence_lock:
            sequence[0] += 1
            seq = sequence[0]
        pair = pairs[seq % len(pairs)]
        check = checks[local_index % len(checks)]
        local_index += 1
        sample_queue.put(run_check(base_url, token, pair, check, seq))


def summarize(samples: list[Sample], duration_seconds: float, max_error_rate: float, target_rps: float) -> dict:
    counts = Counter(sample.name for sample in samples)
    errors = [sample for sample in samples if not sample.ok]
    latencies = [sample.latency_ms for sample in samples]
    endpoint_latencies: dict[str, list[float]] = defaultdict(list)
    for sample in samples:
        endpoint_latencies[sample.name].append(sample.latency_ms)
    total = len(samples)
    error_rate = (len(errors) / total) if total else 1.0
    achieved_rps = total / duration_seconds if duration_seconds > 0 else 0.0
    endpoint_summary = {
        name: {
            "count": len(values),
            "p50_ms": round(statistics.median(values), 2) if values else 0.0,
            "p95_ms": round(percentile(values, 95), 2),
            "p99_ms": round(percentile(values, 99), 2),
        }
        for name, values in sorted(endpoint_latencies.items())
    }
    verdict = "GREEN" if total > 0 and error_rate <= max_error_rate else "RED"
    return {
        "verdict": verdict,
        "target_rps": target_rps,
        "achieved_rps": round(achieved_rps, 2),
        "duration_seconds": round(duration_seconds, 2),
        "total_requests": total,
        "error_count": len(errors),
        "error_rate": round(error_rate, 4),
        "latency": {
            "p50_ms": round(statistics.median(latencies), 2) if latencies else 0.0,
            "p95_ms": round(percentile(latencies, 95), 2),
            "p99_ms": round(percentile(latencies, 99), 2),
            "max_ms": round(max(latencies), 2) if latencies else 0.0,
        },
        "endpoint_counts": dict(sorted(counts.items())),
        "endpoint_latency": endpoint_summary,
        "error_labels": dict(Counter(sample.error or f"http_{sample.status}" for sample in errors).most_common(10)),
        "executed": False,
        "receipt_created": False,
        "source_refs": [
            "/v1/identity/agents",
            "/v1/passports",
            "/v1/passports:verify",
            "/v1/enforcement/scope:evaluate",
            "/v1/policy/evaluations",
            "/v1/passports/by-agent/{agent_id}",
        ],
    }


def write_reports(summary: dict, run_id: str) -> None:
    run_dir = REPORT_DIR / run_id
    run_dir.mkdir(parents=True, exist_ok=True)
    (run_dir / "summary.json").write_text(json.dumps(summary, indent=2), encoding="utf-8")
    (run_dir / "public-summary.md").write_text(
        "\n".join(
            [
                "# ARE Foundation Pressure Test",
                "",
                f"Verdict: **{summary['verdict']}**",
                "",
                f"- Target RPS: `{summary['target_rps']}`",
                f"- Achieved RPS: `{summary['achieved_rps']}`",
                f"- Requests: `{summary['total_requests']}`",
                f"- Errors: `{summary['error_count']}`",
                f"- p95 latency: `{summary['latency']['p95_ms']}ms`",
                f"- p99 latency: `{summary['latency']['p99_ms']}ms`",
                "- Executed: `false`",
                "- Receipt created: `false`",
                "",
                "## Endpoint Mix",
                "",
                *[f"- `{name}`: `{count}`" for name, count in summary["endpoint_counts"].items()],
                "",
            ]
        ),
        encoding="utf-8",
    )
    latest = REPORT_DIR / "latest-summary.json"
    latest.write_text(json.dumps(summary, indent=2), encoding="utf-8")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Pressure test the public ARE Foundation authority path.")
    parser.add_argument("--base-url", default=DEFAULT_BASE, help="Gateway base URL. Defaults to ARE_FOUNDATION_BASE_URL or localhost:18085.")
    parser.add_argument("--token", default=DEFAULT_TOKEN, help="Bearer token. Defaults to ARE_FOUNDATION_TOKEN or test-token.")
    parser.add_argument("--target-rps", type=float, default=10.0)
    parser.add_argument("--duration-seconds", type=float, default=30.0)
    parser.add_argument("--concurrency", type=int, default=8)
    parser.add_argument("--authority-pool-size", type=int, default=8)
    parser.add_argument("--max-error-rate", type=float, default=0.0)
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    run_id = time.strftime("%Y%m%dT%H%M%SZ", time.gmtime())
    pairs = setup_authority_pool(args.base_url.rstrip("/"), args.token, args.authority_pool_size, run_id)
    sample_queue: queue.Queue[Sample] = queue.Queue()
    checks = ["verify", "scope", "policy", "list_passports"]
    limiter = RateLimiter(args.target_rps)
    stop_at = time.perf_counter() + args.duration_seconds
    threads: list[threading.Thread] = []
    sequence_lock = threading.Lock()
    sequence = [0]
    started = time.perf_counter()
    for _ in range(max(1, args.concurrency)):
        thread = threading.Thread(
            target=worker,
            args=(args.base_url.rstrip("/"), args.token, pairs, checks, limiter, stop_at, sample_queue, sequence_lock, sequence),
            daemon=True,
        )
        thread.start()
        threads.append(thread)
    for thread in threads:
        thread.join()
    elapsed = time.perf_counter() - started
    samples = list(sample_queue.queue)
    summary = summarize(samples, elapsed, args.max_error_rate, args.target_rps)
    summary["authority_pool_size"] = len(pairs)
    summary["concurrency"] = args.concurrency
    summary["run_id"] = run_id
    write_reports(summary, run_id)
    print(json.dumps(summary, indent=2))
    return 0 if summary["verdict"] == "GREEN" else 1


if __name__ == "__main__":
    raise SystemExit(main())
