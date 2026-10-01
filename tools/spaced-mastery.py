#!/usr/bin/env python3

import glob
import hashlib
import json
import os
import tempfile
from datetime import datetime, timezone

ROOT = os.path.dirname(
    os.path.dirname(os.path.abspath(__file__))
)

RESULT_DIR = os.path.join(
    ROOT,
    ".local/spaced-results",
)

STATE = os.path.join(
    ROOT,
    ".local/spaced-mastery.json",
)


def atomic_json(path, obj):

    os.makedirs(
        os.path.dirname(path),
        exist_ok=True,
    )

    fd, tmp = tempfile.mkstemp(
        dir=os.path.dirname(path),
        prefix=".mastery-",
        text=True,
    )

    try:
        with os.fdopen(
            fd,
            "w",
            encoding="utf-8",
        ) as f:

            json.dump(
                obj,
                f,
                ensure_ascii=False,
                indent=2,
            )

            f.write("\n")
            f.flush()
            os.fsync(f.fileno())

        os.replace(tmp, path)

    finally:
        if os.path.exists(tmp):
            os.unlink(tmp)


def card_id(result, number):

    cursor = result["cursor"]

    raw = (
        f"{cursor['pass']}:"
        f"{cursor['unit']}:"
        f"{cursor['micro']}:"
        f"{number}"
    )

    return hashlib.sha256(
        raw.encode()
    ).hexdigest()[:24]


def load_state():

    if os.path.exists(STATE):

        with open(
            STATE,
            encoding="utf-8",
        ) as f:
            return json.load(f)

    return {
        "schema_version": 1,
        "processed_results": [],
        "cards": {},
        "updated_at": None,
    }


state = load_state()

processed = set(
    state["processed_results"]
)


for path in sorted(
    glob.glob(
        os.path.join(
            RESULT_DIR,
            "pass-*.json",
        )
    )
):

    name = os.path.basename(path)

    if name in processed:
        continue

    with open(
        path,
        encoding="utf-8",
    ) as f:
        result = json.load(f)

    if result.get("status") != "COMPLETE":
        continue

    for item in result.get(
        "cards",
        [],
    ):

        number = item["card"]

        cid = card_id(
            result,
            number,
        )

        record = state["cards"].get(
            cid,
            {
                "attempts": 0,
                "first_exact_total": 0,
                "regenerated_exact_total": 0,
                "consecutive_first_exact": 0,
                "mastered": False,
                "history": [],
            },
        )

        first_exact = bool(
            item.get("first_exact")
        )

        regenerated_exact = (
            item.get(
                "regenerated_exact"
            ) is True
        )

        record["attempts"] += 1

        if first_exact:
            record["first_exact_total"] += 1
            record[
                "consecutive_first_exact"
            ] += 1
        else:
            record[
                "consecutive_first_exact"
            ] = 0

        if regenerated_exact:
            record[
                "regenerated_exact_total"
            ] += 1

        # Mastery requires two consecutive
        # blind exact recalls.
        if (
            record[
                "consecutive_first_exact"
            ] >= 2
        ):
            record["mastered"] = True

        record["history"].append({
            "pass":
                result["cursor"]["pass"],

            "unit":
                result["cursor"]["unit"],

            "micro":
                result["cursor"]["micro"],

            "card":
                number,

            "first_exact":
                first_exact,

            "regenerated_exact":
                regenerated_exact,
        })

        state["cards"][cid] = record

    state["processed_results"].append(
        name
    )

state["updated_at"] = (
    datetime.now(
        timezone.utc
    ).isoformat()
)

atomic_json(
    STATE,
    state,
)


cards = list(
    state["cards"].values()
)

summary = {
    "tracked_cards":
        len(cards),

    "mastered":
        sum(
            1
            for c in cards
            if c["mastered"]
        ),

    "not_mastered":
        sum(
            1
            for c in cards
            if not c["mastered"]
        ),

    "first_exact_attempts":
        sum(
            c["first_exact_total"]
            for c in cards
        ),

    "regenerated_exact_attempts":
        sum(
            c[
                "regenerated_exact_total"
            ]
            for c in cards
        ),

    "processed_results":
        len(
            state[
                "processed_results"
            ]
        ),
}

print(
    json.dumps(
        summary,
        indent=2,
    )
)
