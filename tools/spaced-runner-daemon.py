#!/usr/bin/env python3

import json
import os
import signal
import subprocess
import time
from datetime import datetime, timezone

ROOT = os.path.dirname(
    os.path.dirname(os.path.abspath(__file__))
)

RUNNER = os.path.join(ROOT, "tools/spaced-api-runner-live.py")
PROGRESS = os.path.join(ROOT, "tools/spaced-progress.py")

# One complete micro should normally finish well below this.
MICRO_TIMEOUT = int(os.environ.get("MICRO_TIMEOUT_SECONDS", "420"))

MIN_BACKOFF = 5
MAX_BACKOFF = 300

stopping = False


def stop_handler(*_):
    global stopping
    stopping = True


signal.signal(signal.SIGTERM, stop_handler)
signal.signal(signal.SIGINT, stop_handler)


def progress(*args):
    return subprocess.run(
        ["python3", PROGRESS, *args],
        cwd=ROOT,
        capture_output=True,
        text=True,
    )


def cursor():
    p = progress("next")

    if p.returncode != 0:
        return None

    try:
        return json.loads(p.stdout)["cursor"]
    except Exception:
        return None


def log(event, **fields):
    payload = {
        "time": datetime.now(timezone.utc).isoformat(),
        "event": event,
        **fields,
    }

    print(
        json.dumps(payload, ensure_ascii=False),
        flush=True,
    )


backoff = MIN_BACKOFF


while not stopping:

    current = cursor()

    if current is None:
        log("TRAINING_COMPLETE")
        break

    log(
        "MICRO_START",
        cursor=current,
        timeout_seconds=MICRO_TIMEOUT,
    )

    started = time.monotonic()

    try:
        result = subprocess.run(
            ["python3", RUNNER],
            cwd=ROOT,
            timeout=MICRO_TIMEOUT,
        )

        rc = result.returncode

    except subprocess.TimeoutExpired:

        elapsed = int(time.monotonic() - started)

        log(
            "MICRO_TIMEOUT",
            cursor=current,
            elapsed_seconds=elapsed,
        )

        # The child was terminated by subprocess timeout.
        # Keep this same cursor for a clean retry.
        progress("mark", "RETRYABLE")

        time.sleep(backoff)
        backoff = min(backoff * 2, MAX_BACKOFF)
        continue

    elapsed = int(time.monotonic() - started)

    if rc == 0:
        log(
            "MICRO_FINISHED",
            cursor=current,
            elapsed_seconds=elapsed,
        )

        backoff = MIN_BACKOFF
        time.sleep(2)
        continue

    if rc == 75:
        log("CREDIT_EXHAUSTED")

        # No aggressive retry loop.
        time.sleep(900)
        continue

    if rc == 76:
        log(
            "TRANSIENT_FAILURE",
            backoff_seconds=backoff,
        )

        time.sleep(backoff)
        backoff = min(backoff * 2, MAX_BACKOFF)
        continue

    log(
        "UNEXPECTED_FAILURE",
        returncode=rc,
        backoff_seconds=backoff,
    )

    # Ensure unknown failure cannot silently advance.
    progress("mark", "RETRYABLE")

    time.sleep(backoff)
    backoff = min(backoff * 2, MAX_BACKOFF)
