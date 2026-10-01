#!/usr/bin/env python3

import json
import sys
import os

ROOT = os.path.abspath(
    os.path.join(
        os.path.dirname(__file__),
        "..",
    )
)

os.chdir(ROOT)


if len(sys.argv) != 2:
    raise SystemExit(
        "usage: "
        "rehearsal-unit-info.py "
        "UNIT_1_TO_26"
    )


unit = int(
    sys.argv[1]
)


with open(
    "docs/ACTIVE_RECALL_REHEARSAL_MANIFEST.json",
    encoding="utf-8",
) as f:

    items = json.load(f)["items"]


if not (
    1 <= unit <= len(items)
):
    raise SystemExit(
        "unit out of range"
    )


item = items[
    unit - 1
]


result = {
    "unit":
        unit,

    "total":
        len(items),

    "kind":
        item["kind"],

    "pack":
        item["pack"],

    "recall":
        item["recall"],

    "correction":
        item["correction"],

    "questions":
        item["questions"],
}


print(
    json.dumps(
        result,
        ensure_ascii=False,
        separators=(",", ":"),
    )
)
