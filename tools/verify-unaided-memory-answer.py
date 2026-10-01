#!/usr/bin/env python3

import json
import hashlib
import sys


if len(sys.argv) != 2:
    raise SystemExit(
        "usage: "
        "verify-unaided-memory-answer.py "
        "ANSWERS.json"
    )


path = sys.argv[1]


with open(
    path,
    encoding="utf-8",
) as f:

    document = json.load(f)


answers = document.get(
    "answers"
)


if not isinstance(
    answers,
    dict,
):
    raise SystemExit(
        "FAIL: answers must be an object"
    )


expected = {
    str(i)
    for i in range(1, 81)
}


actual = set(
    answers
)


missing = expected - actual
extra = actual - expected


if missing or extra:

    print(
        "FAIL: invalid answer IDs"
    )

    print(
        "missing:",
        sorted(
            missing,
            key=int,
        )
    )

    print(
        "extra:",
        sorted(extra)
    )

    raise SystemExit(1)


sha = hashlib.sha256(
    open(
        path,
        "rb",
    ).read()
).hexdigest()


print(
    "UNAIDED ANSWER STRUCTURE PASS"
)

print(
    "count=80"
)

print(
    "sha256=" + sha
)
