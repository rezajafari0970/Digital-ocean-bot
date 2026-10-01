#!/usr/bin/env python3

import json
import random
import os


def load(path):
    with open(path, encoding="utf-8") as f:
        return json.load(f)


def save(path, obj):
    with open(path, "w", encoding="utf-8") as f:
        json.dump(
            obj,
            f,
            ensure_ascii=False,
            separators=(",", ":"),
        )
        f.write("\n")


brain = load(
    "docs/SOURCE_INTERNALIZATION_BRAIN.json"
)

topology = load(
    "docs/SYMBOL_TOPOLOGY_MEMORY_PACK.json"
)

source_manifest = load(
    "docs/SOURCE_MEMORY_PACK_MANIFEST.json"
)

OUT = "docs/spaced-source-rehearsal"

os.makedirs(
    OUT,
    exist_ok=True,
)


# =========================================================
# SOURCE-WIDE CONCEPTUAL + TOPOLOGY CARDS
# =========================================================

base = []


for item in brain["symbol_memory"]:

    base.append({
        "kind":
            "conceptual",

        "cue": {
            "symbol":
                item["symbol"],

            "file":
                item["file"],
        },

        "answer": {
            key: item[key]
            for key in (
                "package",
                "previous_symbol",
                "next_symbol",
                "calls",
                "db_reads",
                "db_writes",
                "routes",
                "side_effects",
            )
        },
    })


for item in topology["files"]:

    base.append({
        "kind":
            "topology",

        "cue": {
            "file":
                item["file"],
        },

        "answer": {
            "symbols":
                item["symbols"]
        },
    })


for item in topology["routes"]:

    base.append({
        "kind":
            "topology",

        "cue": {
            "method":
                item["method"],

            "path":
                item["path"],
        },

        "answer": {
            "handler":
                item["handler"]
        },
    })


passes = []


# =========================================================
# THREE SHUFFLED SOURCE-WIDE PASSES
# =========================================================

for pass_no in range(1, 4):

    cards = list(base)

    start = (
        (pass_no - 1) * 8
    )

    seed_part = (
        brain["source_commit"][
            start:start + 8
        ]
    )

    seed = (
        int(seed_part, 16)
        ^ pass_no
    )

    random.Random(
        seed
    ).shuffle(cards)

    unit_paths = []

    CHUNK = 12

    for unit_no, start_index in enumerate(
        range(
            0,
            len(cards),
            CHUNK,
        ),
        1,
    ):

        chunk = cards[
            start_index:
            start_index + CHUNK
        ]

        path = (
            f"{OUT}/"
            f"pass-{pass_no}-"
            f"unit-{unit_no:03d}.json"
        )

        save(
            path,
            chunk,
        )

        unit_paths.append(
            path
        )

    passes.append({
        "pass":
            pass_no,

        "domain":
            "conceptual_topology",

        "cards":
            len(cards),

        "units":
            unit_paths,
    })


# =========================================================
# VERBATIM CARDS
#
# Uses trained first 10 packs.
# Cue includes first line.
# Answer requires complete block.
# =========================================================

verbatim_cards = []


for pack in source_manifest[
    "verbatim_packs"
][:10]:

    blocks = load(
        pack["path"]
    )

    for block in blocks:

        text = block["text"]

        if not text.strip():
            continue

        first_line = (
            text.split("\n")[0]
        )

        verbatim_cards.append({
            "kind":
                "verbatim",

            "cue": {
                "file":
                    block["file"],

                "start_line":
                    block["start_line"],

                "end_line":
                    block["end_line"],

                "first_line":
                    first_line,
            },

            "answer": {
                "text":
                    text
            },
        })


for pass_no in range(1, 4):

    cards = list(
        verbatim_cards
    )

    random.Random(
        0x565242 + pass_no
    ).shuffle(cards)

    unit_paths = []

    CHUNK = 8

    for unit_no, start_index in enumerate(
        range(
            0,
            len(cards),
            CHUNK,
        ),
        1,
    ):

        chunk = cards[
            start_index:
            start_index + CHUNK
        ]

        path = (
            f"{OUT}/"
            f"verbatim-pass-{pass_no}-"
            f"unit-{unit_no:03d}.json"
        )

        save(
            path,
            chunk,
        )

        unit_paths.append(
            path
        )

    passes.append({
        "pass":
            f"V{pass_no}",

        "domain":
            "verbatim",

        "cards":
            len(cards),

        "units":
            unit_paths,
    })


# =========================================================
# MANIFEST
# =========================================================

manifest = {
    "schema_version":
        1,

    "source_commit":
        brain["source_commit"],

    "policy": (
        "Three shuffled passes. "
        "Failed cards recur. "
        "A card may retire only after "
        "two consecutive exact recalls."
    ),

    "source_wide_cards":
        len(base),

    "verbatim_cards":
        len(verbatim_cards),

    "passes":
        passes,
}


save(
    "docs/SPACED_SOURCE_REHEARSAL_MANIFEST.json",
    manifest,
)


print(
    "Source-wide cards:",
    len(base),
)

print(
    "Verbatim cards:",
    len(verbatim_cards),
)

print(
    "Passes:",
    len(passes),
)

print(
    "Total units:",
    sum(
        len(x["units"])
        for x in passes
    ),
)
