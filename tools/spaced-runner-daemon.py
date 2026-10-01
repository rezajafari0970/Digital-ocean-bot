#!/usr/bin/env python3

import json
import os
import signal
import subprocess
import time

ROOT = os.path.dirname(
    os.path.dirname(os.path.abspath(__file__))
)

RUNNER = os.path.join(
    ROOT,
    "tools/spaced-api-runner-live.py",
)

PROGRESS = os.path.join(
    ROOT,
    "tools/spaced-progress.py",
)

stopping = False


def stop_handler(*_):
    global stopping
    stopping = True


signal.signal(
    signal.SIGTERM,
    stop_handler,
)

signal.signal(
    signal.SIGINT,
    stop_handler,
)


def get_cursor():

    p = subprocess.run(
        [
            "python3",
            PROGRESS,
            "next",
        ],
        cwd=ROOT,
        capture_output=True,
        text=True,
    )

    if p.returncode != 0:
        return None

    try:
        return json.loads(
            p.stdout
        ).get("cursor")

    except Exception:
        return None


backoff = 5


while not stopping:

    cursor = get_cursor()

    if cursor is None:

        print(
            "TRAINING_COMPLETE",
            flush=True,
        )

        break

    print(
        "RUN",
        json.dumps(
            cursor,
            ensure_ascii=False,
        ),
        flush=True,
    )

    result = subprocess.run(
        [
            "python3",
            RUNNER,
        ],
        cwd=ROOT,
    )

    rc = result.returncode

    # Successful micro or safely handled block.
    if rc == 0:

        backoff = 5

        time.sleep(2)

        continue

    # Credit exhausted.
    if rc == 75:

        print(
            "PAUSED_CREDIT_EXHAUSTED",
            flush=True,
        )

        # Check again every 15 minutes.
        time.sleep(900)

        continue

    # 429 / transient API / server error.
    if rc == 76:

        print(
            "RETRYABLE_BACKOFF",
            backoff,
            flush=True,
        )

        time.sleep(backoff)

        backoff = min(
            backoff * 2,
            300,
        )

        continue

    # Unexpected runner failure.
    print(
        "RUNNER_FAILURE",
        rc,
        "BACKOFF",
        backoff,
        flush=True,
    )

    time.sleep(backoff)

    backoff = min(
        backoff * 2,
        300,
    )
