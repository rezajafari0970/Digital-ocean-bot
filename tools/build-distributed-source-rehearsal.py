#!/usr/bin/env python3

import json
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

OUT = "docs/distributed-source-rehearsal"

os.makedirs(
    OUT,
    exist_ok=True,
)

units = []


# =========================================================
# CONCEPTUAL — ENTIRE SYMBOL MEMORY
# =========================================================

symbols = brain["symbol_memory"]

CHUNK = 15

for unit_no, start in enumerate(
    range(0, len(symbols), CHUNK),
    1,
):

    cards = []

    for item in symbols[
        start:start + CHUNK
    ]:

        cards.append({
            "symbol":
                item["symbol"],

            "file":
                item["file"],

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

    path = (
        f"{OUT}/"
        f"concept-{unit_no:03d}.json"
    )

    save(
        path,
        cards,
    )

    units.append({
        "kind":
            "conceptual",

        "path":
            path,

        "cards":
            len(cards),
    })


# =========================================================
# TOPOLOGY — EVERY INDEXED FILE
# =========================================================

topology_cards = []

for item in topology["files"]:

    topology_cards.append({
        "type":
            "file",

        "file":
            item["file"],

        "answer": {
            "symbols":
                item["symbols"]
        },
    })


# =========================================================
# TOPOLOGY — EVERY INDEXED ROUTE
# =========================================================

for item in topology["routes"]:

    topology_cards.append({
        "type":
            "route",

        "method":
            item["method"],

        "path":
            item["path"],

        "answer": {
            "handler":
                item["handler"]
        },
    })


for unit_no, start in enumerate(
    range(
        0,
        len(topology_cards),
        CHUNK,
    ),
    1,
):

    cards = topology_cards[
        start:start + CHUNK
    ]

    path = (
        f"{OUT}/"
        f"topology-{unit_no:03d}.json"
    )

    save(
        path,
        cards,
    )

    units.append({
        "kind":
            "topology",

        "path":
            path,

        "cards":
            len(cards),
    })


# =========================================================
# VERBATIM
#
# Previous training = first 5 packs.
# New distributed phase = first 10 packs.
# =========================================================

verbatim_packs = (
    source_manifest[
        "verbatim_packs"
    ][:10]
)


for unit_no, pack in enumerate(
    verbatim_packs,
    1,
):

    units.append({
        "kind":
            "verbatim",

        "path":
            pack["path"],

        "cards":
            pack.get(
                "blocks",
                50,
            ),
    })


# =========================================================
# MANIFEST
# =========================================================

manifest = {
    "schema_version":
        1,

    "source_commit":
        brain["source_commit"],

    "conceptual_symbol_count":
        len(symbols),

    "topology_card_count":
        len(topology_cards),

    "verbatim_pack_count":
        len(verbatim_packs),

    "units":
        units,
}


save(
    "docs/DISTRIBUTED_SOURCE_REHEARSAL_MANIFEST.json",
    manifest,
)


print(
    "Conceptual symbols:",
    len(symbols),
)

print(
    "Topology cards:",
    len(topology_cards),
)

print(
    "Verbatim packs:",
    len(verbatim_packs),
)

print(
    "Total rehearsal units:",
    len(units),
)
