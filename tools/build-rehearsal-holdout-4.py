#!/usr/bin/env python3

import json
import random
import os
import hashlib


def load(path):
    with open(path, encoding="utf-8") as f:
        return json.load(f)


brain = load("docs/SOURCE_INTERNALIZATION_BRAIN.json")
topology = load("docs/SYMBOL_TOPOLOGY_MEMORY_PACK.json")
manifest = load("docs/SOURCE_MEMORY_PACK_MANIFEST.json")

h2 = load("docs/REHEARSAL_HOLDOUT_2_EXAM.json")
h3 = load("docs/REHEARSAL_HOLDOUT_3_EXAM.json")


# Deterministic but different from H2/H3.
seed = (
    int(brain["source_commit"][8:20], 16)
    ^ 0x484F4C4434
)

rng = random.Random(seed)


def identity(metric, prompt):
    # Extract backtick-delimited identity where possible.
    if "`" in prompt:
        parts = prompt.split("`")

        if len(parts) >= 3:
            return (
                metric,
                parts[1],
            )

    return (
        metric,
        prompt,
    )


used = set()

for exam in (h2, h3):
    for q in exam["questions"]:
        used.add(
            identity(
                q["metric"],
                q["prompt"],
            )
        )


questions = []
keys = {}


def add(metric, prompt, answer):
    ident = identity(
        metric,
        prompt,
    )

    if ident in used:
        return False

    number = len(questions) + 1

    questions.append({
        "id": number,
        "metric": metric,
        "prompt": prompt,
    })

    keys[str(number)] = answer

    used.add(ident)

    return True


# =========================================================
# 30 CONCEPTUAL
# =========================================================

symbols = list(
    brain["symbol_memory"]
)

rng.shuffle(symbols)

conceptual_count = 0


for item in symbols:

    if conceptual_count >= 30:
        break

    if not (
        item["calls"]
        or item["db_reads"]
        or item["db_writes"]
        or item["routes"]
        or item["side_effects"]
    ):
        continue

    prompt = (
        "Exact structured recall for symbol "
        f"`{item['symbol']}` in file "
        f"`{item['file']}`."
    )

    answer = {
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
    }

    if add(
        "conceptual",
        prompt,
        answer,
    ):
        conceptual_count += 1


# =========================================================
# 20 FILE → ORDERED SYMBOLS
# =========================================================

files = [
    item
    for item in topology["files"]
    if len(item["symbols"]) >= 2
]

rng.shuffle(files)

topology_count = 0


for item in files:

    if topology_count >= 20:
        break

    prompt = (
        "Exact ordered-symbol topology "
        f"for file `{item['file']}`."
    )

    if add(
        "topology",
        prompt,
        {
            "symbols":
                item["symbols"]
        },
    ):
        topology_count += 1


# =========================================================
# 10 ROUTE → HANDLER
# =========================================================

routes = list(
    topology["routes"]
)

rng.shuffle(routes)


for item in routes:

    if topology_count >= 30:
        break

    route_identity = (
        f"{item['method']} "
        f"{item['path']}"
    )

    prompt = (
        "Exact route handler for "
        f"`{route_identity}`."
    )

    if add(
        "topology",
        prompt,
        {
            "handler":
                item["handler"]
        },
    ):
        topology_count += 1


# =========================================================
# 20 VERBATIM
# Same declared Cohort-1 coverage.
# =========================================================

blocks = []

for pack in manifest[
    "verbatim_packs"
][:5]:

    blocks.extend(
        load(
            pack["path"]
        )
    )


rng.shuffle(blocks)

verbatim_count = 0


for block in blocks:

    if verbatim_count >= 20:
        break

    if not block["text"].strip():
        continue

    line_count = (
        block["end_line"]
        - block["start_line"]
        + 1
    )

    if line_count < 5:
        continue

    source_identity = (
        f"{block['file']}:"
        f"{block['start_line']}-"
        f"{block['end_line']}"
    )

    prompt = (
        "Exact source block "
        f"`{source_identity}`."
    )

    if add(
        "verbatim",
        prompt,
        {
            "text":
                block["text"]
        },
    ):
        verbatim_count += 1


assert conceptual_count == 30, conceptual_count
assert topology_count == 30, topology_count
assert verbatim_count == 20, verbatim_count
assert len(questions) == 80


# =========================================================
# PRIVATE KEY
# =========================================================

audit_dir = (
    ".audit/rehearsal-holdout-4"
)

os.makedirs(
    audit_dir,
    exist_ok=True,
)

key_path = (
    audit_dir + "/key.json"
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
# PUBLIC EXAM
# =========================================================

exam_path = (
    "docs/REHEARSAL_HOLDOUT_4_EXAM.json"
)


public = {
    "schema_version": 1,

    "source_commit":
        brain["source_commit"],

    "question_count":
        80,

    "answer_contract": {

        "conceptual": [
            "package",
            "previous_symbol",
            "next_symbol",
            "calls",
            "db_reads",
            "db_writes",
            "routes",
            "side_effects",
        ],

        "topology":
            "exact symbols[] or handler",

        "verbatim":
            "exact text",
    },

    "questions":
        questions,
}


with open(
    exam_path,
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


print("HOLDOUT 4 READY")

print(
    "Conceptual:",
    conceptual_count,
)

print(
    "Topology:",
    topology_count,
)

print(
    "Verbatim:",
    verbatim_count,
)

print(
    "Exam SHA256:",
    hashlib.sha256(
        open(
            exam_path,
            "rb",
        ).read()
    ).hexdigest(),
)

print(
    "Key SHA256:",
    hashlib.sha256(
        open(
            key_path,
            "rb",
        ).read()
    ).hexdigest(),
)
