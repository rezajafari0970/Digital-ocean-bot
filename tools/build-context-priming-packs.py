#!/usr/bin/env python3

import json
import os
import hashlib

ROOT = os.path.abspath(
    os.path.join(
        os.path.dirname(__file__),
        "..",
    )
)

os.chdir(ROOT)

with open(
    "docs/SOURCE_INTERNALIZATION_BRAIN.json",
    encoding="utf-8",
) as f:
    brain = json.load(f)

with open(
    "docs/SYMBOL_TOPOLOGY_MEMORY_PACK.json",
    encoding="utf-8",
) as f:
    topology = json.load(f)


OUT = "docs/source-memory-packs"

os.makedirs(
    OUT,
    exist_ok=True,
)


manifest = {
    "schema_version": 1,
    "source_commit": brain["source_commit"],
    "conceptual_packs": [],
    "topology_packs": [],
    "verbatim_packs": [],
}


def write_pack(
    kind,
    number,
    payload,
):

    path = (
        f"{OUT}/"
        f"{kind}-{number:03d}.json"
    )

    data = (
        json.dumps(
            payload,
            ensure_ascii=False,
            separators=(",", ":"),
        )
        + "\n"
    )

    with open(
        path,
        "w",
        encoding="utf-8",
    ) as f:
        f.write(data)

    encoded = data.encode("utf-8")

    return {
        "path": path,
        "sha256": hashlib.sha256(
            encoded
        ).hexdigest(),
        "bytes": len(encoded),
    }


# =========================================================
# CONCEPTUAL MEMORY PACKS
# =========================================================

symbols = brain["symbol_memory"]

CONCEPT_SIZE = 120

for start in range(
    0,
    len(symbols),
    CONCEPT_SIZE,
):

    chunk = []

    for item in symbols[
        start:start + CONCEPT_SIZE
    ]:

        chunk.append({
            "file":
                item["file"],

            "symbol":
                item["symbol"],

            "kind":
                item["kind"],

            "package":
                item["package"],

            "receiver":
                item["receiver"],

            "start_line":
                item["start_line"],

            "end_line":
                item["end_line"],

            "signature":
                item["signature"],

            "previous_symbol":
                item["previous_symbol"],

            "next_symbol":
                item["next_symbol"],

            "calls":
                item["calls"],

            "db_reads":
                item["db_reads"],

            "db_writes":
                item["db_writes"],

            "routes":
                item["routes"],

            "side_effects":
                item["side_effects"],

            "structural_role":
                item["structural_role"],
        })

    number = (
        start // CONCEPT_SIZE
    ) + 1

    manifest[
        "conceptual_packs"
    ].append(
        write_pack(
            "concept",
            number,
            chunk,
        )
    )


# =========================================================
# TOPOLOGY / NAME PACKS
# =========================================================

files = topology["files"]

TOPOLOGY_SIZE = 55

for start in range(
    0,
    len(files),
    TOPOLOGY_SIZE,
):

    number = (
        start // TOPOLOGY_SIZE
    ) + 1

    payload = {
        "files":
            files[
                start:
                start + TOPOLOGY_SIZE
            ],

        "routes":
            (
                topology["routes"]
                if start == 0
                else []
            ),
    }

    manifest[
        "topology_packs"
    ].append(
        write_pack(
            "topology",
            number,
            payload,
        )
    )


# =========================================================
# VERBATIM SOURCE PACKS
# =========================================================

blocks = brain["source_blocks"]

VERBATIM_SIZE = 50

for start in range(
    0,
    len(blocks),
    VERBATIM_SIZE,
):

    number = (
        start // VERBATIM_SIZE
    ) + 1

    manifest[
        "verbatim_packs"
    ].append(
        write_pack(
            "verbatim",
            number,
            blocks[
                start:
                start + VERBATIM_SIZE
            ],
        )
    )


with open(
    "docs/SOURCE_MEMORY_PACK_MANIFEST.json",
    "w",
    encoding="utf-8",
) as f:

    json.dump(
        manifest,
        f,
        ensure_ascii=False,
        indent=2,
    )

    f.write("\n")


print(
    "CONCEPTUAL PACKS:",
    len(
        manifest[
            "conceptual_packs"
        ]
    ),
)

print(
    "TOPOLOGY PACKS:",
    len(
        manifest[
            "topology_packs"
        ]
    ),
)

print(
    "VERBATIM PACKS:",
    len(
        manifest[
            "verbatim_packs"
        ]
    ),
)

print(
    "TOTAL BYTES:",
    sum(
        item["bytes"]
        for category in (
            "conceptual_packs",
            "topology_packs",
            "verbatim_packs",
        )
        for item in manifest[
            category
        ]
    ),
)

