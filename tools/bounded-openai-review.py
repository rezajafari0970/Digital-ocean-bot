#!/usr/bin/env python3
"""One bounded Astra review. Never executes model output or retries a request."""
import fcntl
import hashlib
import json
import os
from pathlib import Path
import sys
import tempfile
import time
import urllib.request
import uuid

MODEL = "gpt-6-astra"
# Standard text prices verified 2026-10-25, USD per token in millionths.
# Cached input is conservatively charged as uncached; tools/fast mode are disabled.
INPUT_MICRO = 10
OUTPUT_MICRO = 50
MAX_INPUT_BYTES = 64000
MAX_OUTPUT = 16384
BUDGET_MICRO = 2000000
MAX_REQUESTS = 3

class ReviewBlocked(RuntimeError):
    pass

def atomic_write(path, data):
    fd, name = tempfile.mkstemp(prefix=".budget-", dir=path.parent)
    try:
        with os.fdopen(fd, "w") as stream:
            os.fchmod(stream.fileno(), 0o600)
            json.dump(data, stream, sort_keys=True)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(name, path)
        fd = os.open(path.parent, os.O_DIRECTORY)
        try:
            os.fsync(fd)
        finally:
            os.close(fd)
    finally:
        if os.path.exists(name):
            os.unlink(name)

def http_send(payload, key):
    request = urllib.request.Request(
        "https://api.openai.com/v1/responses",
        json.dumps(payload).encode(),
        {"Authorization": "Bearer " + key, "Content-Type": "application/json"},
    )
    with urllib.request.urlopen(request, timeout=300) as response:
        raw = response.read(5000001)
    if len(raw) > 5000000:
        raise ReviewBlocked("response_too_large")
    return json.loads(raw)

def review(prompt, key, path, *, send=http_send):
    if not key:
        raise ReviewBlocked("api_key_missing")
    size = len(prompt.encode())
    if not prompt.strip() or size > MAX_INPUT_BYTES:
        raise ReviewBlocked("input_empty_or_too_large")
    path = Path(path)
    if not path.is_absolute():
        raise ReviewBlocked("absolute_ledger_path_required")
    path.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
    if path.parent.is_symlink() or path.is_symlink():
        raise ReviewBlocked("unsafe_ledger_path")
    if path.parent.stat().st_uid != os.geteuid() or path.parent.stat().st_mode & 0o077:
        raise ReviewBlocked("ledger_directory_must_be_private_and_owned")
    lockpath = path.with_suffix(path.suffix + ".lock")
    fd = os.open(lockpath, os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, "a") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise ReviewBlocked("review_already_running")
        policy = {"model": MODEL, "budget_micro_usd": BUDGET_MICRO,
                  "max_requests": MAX_REQUESTS, "input_micro_per_token": INPUT_MICRO,
                  "output_micro_per_token": OUTPUT_MICRO}
        state = {"schema": 1, "policy": policy, "requests": []}
        if path.exists():
            try:
                state = json.loads(path.read_text())
            except (ValueError, OSError):
                raise ReviewBlocked("budget_ledger_unreadable")
        if state.get("schema") != 1 or state.get("policy") != policy or not isinstance(state.get("requests"), list):
            raise ReviewBlocked("budget_policy_mismatch")
        rows = state["requests"]
        if len(rows) > MAX_REQUESTS or any(
            not isinstance(row, dict) or
            type(row.get("charged_micro_usd")) is not int or
            not 0 <= row["charged_micro_usd"] <= BUDGET_MICRO or
            type(row.get("reserved_micro_usd")) is not int or
            not 0 < row["reserved_micro_usd"] <= BUDGET_MICRO or
            row.get("status") not in ("completed", "reserved", "unknown")
            for row in rows
        ):
            raise ReviewBlocked("budget_ledger_invalid")
        payload = {"model": MODEL, "reasoning": {"effort": "max"},
                   "max_output_tokens": MAX_OUTPUT, "input": prompt,
                   "store": False, "service_tier": "default"}
        signature = hashlib.sha256(json.dumps(payload, sort_keys=True).encode()).hexdigest()
        if any(row.get("status") != "completed" for row in rows):
            raise ReviewBlocked("unresolved_request_check_provider_usage_before_recovery")
        if any(row.get("payload_digest") == signature for row in rows):
            raise ReviewBlocked("duplicate_review")
        if len(rows) >= MAX_REQUESTS:
            raise ReviewBlocked("request_limit_reached")
        # UTF-8 bytes plus framing reserve deliberately overestimate plain-text
        # input tokens. No hidden conversation, tools or images are included.
        reserve = (size + 2048) * INPUT_MICRO + MAX_OUTPUT * OUTPUT_MICRO
        spent = sum(row["charged_micro_usd"] for row in rows)
        if spent + reserve > BUDGET_MICRO:
            raise ReviewBlocked("budget_reservation_exceeds_cap")
        row = {"attempt": str(uuid.uuid4()), "at": time.time(), "status": "reserved",
               "payload_digest": signature, "reserved_micro_usd": reserve,
               "charged_micro_usd": reserve}
        rows.append(row)
        atomic_write(path, state)  # Persist before the network side effect.
        try:
            result = send(payload, key)
            usage = result["usage"]
            if result.get("model") != MODEL and not str(result.get("model", "")).startswith(MODEL + "-"):
                raise ReviewBlocked("unexpected_response_model")
            for name in ("input_tokens", "output_tokens"):
                if type(usage.get(name)) is not int or usage[name] < 0:
                    raise ReviewBlocked("usage_missing_or_invalid")
            cost = usage["input_tokens"] * INPUT_MICRO + usage["output_tokens"] * OUTPUT_MICRO
            if cost > reserve:
                raise ReviewBlocked("usage_exceeds_reserved_bound")
            row.update(status="completed", response_id=result.get("id"),
                       response_status=result.get("status"), usage=usage,
                       charged_micro_usd=cost, model=result.get("model"))
            atomic_write(path, state)
        except Exception as exc:
            # Timeout/HTTP/parse/crash uncertainty never frees a paid reservation.
            row.update(status="unknown", error_type=type(exc).__name__)
            atomic_write(path, state)
            raise ReviewBlocked("request_outcome_uncertain_reservation_retained") from None
        text = "".join(c.get("text", "") for out in result.get("output", [])
                       for c in out.get("content", []) if c.get("type") == "output_text")
        if result.get("status") != "completed" or not text.strip():
            raise ReviewBlocked("response_incomplete_usage_recorded_no_automatic_retry")
        return {"model": result["model"], "response_id": result.get("id"),
                "usage": usage, "estimated_cost_usd_upper": cost / 1000000,
                "ledger_estimated_total_usd_upper": sum(x["charged_micro_usd"] for x in rows) / 1000000,
                "text": text}

def main():
    # A fixed durable ledger prevents accidental per-invocation budget resets.
    path = Path("/var/lib/datacenter-bot-audit/openai/ledger.json")
    try:
        result = review(sys.stdin.read(MAX_INPUT_BYTES + 1), os.environ.get("OPENAI_API_KEY"), path)
    except (ReviewBlocked, OSError) as exc:
        print(json.dumps({"blocked": str(exc) if isinstance(exc, ReviewBlocked) else "ledger_io_failed"}), file=sys.stderr)
        return 78
    print(json.dumps(result, ensure_ascii=False))
    return 0

if __name__ == "__main__":
    raise SystemExit(main())
