#!/usr/bin/env python3

import json
import random
import os
import hashlib

ROOT = os.path.abspath(
    os.path.join(
        os.path.dirname(__file__),
        "..",
    )
)

os.chdir(ROOT)


def load(path):
    with open(path, encoding="utf-8") as f:
        return json.load(f)


brain = load(
    "docs/SOURCE_INTERNALIZATION_BRAIN.json"
)

topology = load(
    "docs/SYMBOL_TOPOLOGY_MEMORY_PACK.json"
)

manifest = load(
    "docs/SOURCE_MEMORY_PACK_MANIFEST.json"
)


rng = random.Random(
    int(
        brain["source_commit"][36:48],
        16,
    )
)


questions = []
keys = {}


def add(
    metric,
    prompt,
    answer,
):

    number = len(questions) + 1

    questions.append({
        "id": number,
        "metric": metric,
        "prompt": prompt,
    })

    keys[str(number)] = answer


# =========================================================
# CONCEPTUAL HOLDOUT — 30
# =========================================================

candidates = [
    x
    for x in brain["symbol_memory"]
    if (
        x["calls"]
        or x["db_reads"]
        or x["db_writes"]
        or x["routes"]
        or x["side_effects"]
    )
]


for item in rng.sample(
    candidates,
    30,
):

    add(
        "conceptual",
        (
            "Closed-book: for "
            f"`{item['symbol']}` in "
            f"`{item['file']}`, give "
            "package, previous/next symbol, "
            "calls, DB reads/writes, routes, "
            "and side effects."
        ),
        {
            "package":
                item["package"],

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
        },
    )


# =========================================================
# TOPOLOGY HOLDOUT — 20 FILE QUESTIONS
# =========================================================

eligible_files = [
    x
    for x in topology["files"]
    if len(x["symbols"]) >= 2
]


for item in rng.sample(
    eligible_files,
    20,
):

    add(
        "topology",
        (
            "Closed-book: list ordered "
            f"symbols in `{item['file']}`."
        ),
        {
            "symbols":
                item["symbols"]
        },
    )


# =========================================================
# TOPOLOGY HOLDOUT — 10 ROUTES
# =========================================================

for route in rng.sample(
    topology["routes"],
    10,
):

    add(
        "topology",
        (
            "Closed-book: handler for "
            f"`{route['method']} "
            f"{route['path']}`?"
        ),
        {
            "handler":
                route["handler"]
        },
    )


# =========================================================
# VERBATIM HOLDOUT — COHORT-1 MATERIAL ONLY
# =========================================================

primed_blocks = []

for pack in manifest[
    "verbatim_packs"
][:5]:

    primed_blocks.extend(
        load(
            pack["path"]
        )
    )


eligible_blocks = [
    x
    for x in primed_blocks
    if (
        x["text"].strip()
        and
        x["end_line"]
        - x["start_line"]
        + 1
        >= 5
    )
]


for block in rng.sample(
    eligible_blocks,
    20,
):

    add(
        "verbatim",
        (
            "Closed-book: reproduce exactly "
            f"lines {block['start_line']}-"
            f"{block['end_line']} of "
            f"`{block['file']}`."
        ),
        {
            "text":
                block["text"]
        },
    )


# =========================================================
# PRIVATE KEY
# =========================================================

os.makedirs(
    ".audit/rehearsal-holdout-2",
    exist_ok=True,
)


key_path = (
    ".audit/rehearsal-holdout-2/"
    "key.json"
)


with open(
    key_path,
    "w",
    encoding="utf-8",
) as f:

    json.dump(
        {
            "source_commit":
                brain["source_commit"],

            "keys":
                keys,
        },
        f,
        ensure_ascii=False,
        indent=2,
    )

    f.write("\n")


# =========================================================
# PUBLIC HOLDOUT
# =========================================================

verbatim_coverage = round(
    100
    * 250
    / brain["source_block_count"],
    2,
)


public = {
    "schema_version": 1,

    "source_commit":
        brain["source_commit"],

    "question_count":
        len(questions),

    "questions":
        questions,

    "verbatim_coverage_percent":
        verbatim_coverage,
}


with open(
    "docs/REHEARSAL_HOLDOUT_2_EXAM.json",
    "w",
    encoding="utf-8",
) as f:

    json.dump(
        public,
        f,
        ensure_ascii=False,
        indent=2,
    )

    f.write("\n")


key_sha = hashlib.sha256(
    open(
        key_path,
        "rb",
    ).read()
).hexdigest()


print(
    "HOLDOUT QUESTIONS:",
    len(questions),
)

print(
    "VERBATIM COVERAGE:",
    verbatim_coverage,
)

print(
    "PRIVATE KEY SHA256:",
    key_sha,
)
