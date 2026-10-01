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
holdout2 = load("docs/REHEARSAL_HOLDOUT_2_EXAM.json")

rng = random.Random(
    int(brain["source_commit"][-12:], 16)
)

used = {
    (q["metric"], q["prompt"])
    for q in holdout2["questions"]
}

questions = []
keys = {}

def add(metric, prompt, answer):
    if (metric, prompt) in used:
        return False

    number = len(questions) + 1

    questions.append({
        "id": number,
        "metric": metric,
        "prompt": prompt,
    })

    keys[str(number)] = answer
    return True


# ==========================================
# 30 CONCEPTUAL — STRUCTURED
# ==========================================

candidates = [
    x for x in brain["symbol_memory"]
    if (
        x["calls"]
        or x["db_reads"]
        or x["db_writes"]
        or x["routes"]
        or x["side_effects"]
    )
]

rng.shuffle(candidates)

conceptual_count = 0

for x in candidates:
    if conceptual_count >= 30:
        break

    prompt = (
        "Closed-book structured recall: "
        f"for `{x['symbol']}` in `{x['file']}`, "
        "return exactly package, previous_symbol, "
        "next_symbol, calls, db_reads, db_writes, "
        "routes, side_effects."
    )

    if add(
        "conceptual",
        prompt,
        {
            k: x[k]
            for k in (
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
    ):
        conceptual_count += 1


# ==========================================
# 20 FILE TOPOLOGY
# ==========================================

files = [
    x for x in topology["files"]
    if len(x["symbols"]) >= 2
]

rng.shuffle(files)

topology_count = 0

for x in files:
    if topology_count >= 20:
        break

    if add(
        "topology",
        (
            "Closed-book exact topology: "
            f"ordered symbols in `{x['file']}`."
        ),
        {
            "symbols": x["symbols"]
        },
    ):
        topology_count += 1


# ==========================================
# 10 ROUTE → HANDLER
# ==========================================

routes = list(topology["routes"])
rng.shuffle(routes)

for x in routes:
    if topology_count >= 30:
        break

    if add(
        "topology",
        (
            "Closed-book exact handler: "
            f"`{x['method']} {x['path']}`."
        ),
        {
            "handler": x["handler"]
        },
    ):
        topology_count += 1


# ==========================================
# 20 VERBATIM
# Only from trained Cohort-1 coverage
# ==========================================

primed = []

for pack in manifest["verbatim_packs"][:5]:
    primed.extend(load(pack["path"]))

rng.shuffle(primed)

verbatim_count = 0

for x in primed:
    if verbatim_count >= 20:
        break

    if not x["text"].strip():
        continue

    if x["end_line"] - x["start_line"] + 1 < 5:
        continue

    if add(
        "verbatim",
        (
            "Closed-book exact source: reproduce "
            f"lines {x['start_line']}-"
            f"{x['end_line']} of `{x['file']}`."
        ),
        {
            "text": x["text"]
        },
    ):
        verbatim_count += 1


assert conceptual_count == 30
assert topology_count == 30
assert verbatim_count == 20
assert len(questions) == 80


# ==========================================
# PRIVATE KEY
# ==========================================

os.makedirs(
    ".audit/rehearsal-holdout-3",
    exist_ok=True,
)

key_path = (
    ".audit/rehearsal-holdout-3/key.json"
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


# ==========================================
# PUBLIC EXAM
# ==========================================

public = {
    "schema_version": 1,

    "source_commit":
        brain["source_commit"],

    "question_count":
        80,

    "answer_contract": {
        "conceptual": (
            "Return an exact object containing "
            "package, previous_symbol, next_symbol, "
            "calls, db_reads, db_writes, routes, "
            "side_effects."
        ),

        "topology": (
            "Return an exact object containing "
            "symbols or handler as requested."
        ),

        "verbatim": (
            "Return an exact object "
            "{\"text\": \"...\"}."
        ),
    },

    "questions":
        questions,
}

exam_path = (
    "docs/REHEARSAL_HOLDOUT_3_EXAM.json"
)

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


print("HOLDOUT 3 READY")
print("Conceptual:", conceptual_count)
print("Topology:", topology_count)
print("Verbatim:", verbatim_count)

print(
    "Exam SHA256:",
    hashlib.sha256(
        open(exam_path, "rb").read()
    ).hexdigest(),
)

print(
    "Private key SHA256:",
    hashlib.sha256(
        open(key_path, "rb").read()
    ).hexdigest(),
)
