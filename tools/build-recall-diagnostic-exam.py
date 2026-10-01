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
        brain["source_commit"][24:36],
        16,
    )
)


questions = []
keys = {}


def add(
    mode,
    domain,
    prompt,
    options,
    answer,
):

    number = len(questions) + 1

    item = {
        "id": number,
        "mode": mode,
        "domain": domain,
        "prompt": prompt,
    }

    if options is not None:
        item["options"] = options

    questions.append(item)

    keys[str(number)] = answer


# =========================================================
# CONCEPTUAL RECOGNITION — 15
# =========================================================

symbol_pool = [
    x
    for x in brain["symbol_memory"]
    if x["signature"]
]

all_files = list({
    x["file"]
    for x in brain["symbol_memory"]
})


for item in rng.sample(
    symbol_pool,
    15,
):

    distractors = rng.sample(
        [
            x
            for x in all_files
            if x != item["file"]
        ],
        3,
    )

    options = [
        item["file"],
        *distractors,
    ]

    rng.shuffle(options)

    add(
        "recognition",
        "conceptual",
        (
            "Which file contains symbol "
            f"`{item['symbol']}`?"
        ),
        options,
        item["file"],
    )


# =========================================================
# ROUTE / TOPOLOGY RECOGNITION — 10
# =========================================================

for route in rng.sample(
    topology["routes"],
    10,
):

    handlers = list({
        x["handler"]
        for x in topology["routes"]
        if x["handler"] != route["handler"]
    })

    distractors = rng.sample(
        handlers,
        min(3, len(handlers)),
    )

    options = [
        route["handler"],
        *distractors,
    ]

    rng.shuffle(options)

    add(
        "recognition",
        "topology",
        (
            "What is the handler for "
            f"`{route['method']} "
            f"{route['path']}`?"
        ),
        options,
        route["handler"],
    )


# =========================================================
# NEXT-SYMBOL RECOGNITION — 5
# =========================================================

eligible_files = [
    x
    for x in topology["files"]
    if len(x["symbols"]) >= 3
]


for file_item in rng.sample(
    eligible_files,
    5,
):

    symbols = file_item["symbols"]

    index = rng.randrange(
        0,
        len(symbols) - 1,
    )

    current = symbols[index]
    answer = symbols[index + 1]

    pool = list({
        symbol
        for f in topology["files"]
        for symbol in f["symbols"]
        if symbol != answer
    })

    distractors = rng.sample(
        pool,
        3,
    )

    options = [
        answer,
        *distractors,
    ]

    rng.shuffle(options)

    add(
        "recognition",
        "topology",
        (
            f"In `{file_item['file']}`, "
            "which symbol immediately "
            f"follows `{current}`?"
        ),
        options,
        answer,
    )


# =========================================================
# LOAD ONLY COHORT-1 VERBATIM MATERIAL
# =========================================================

primed_blocks = []

for pack in manifest[
    "verbatim_packs"
][:5]:

    primed_blocks.extend(
        load(pack["path"])
    )


# =========================================================
# VERBATIM RECOGNITION — 10
# =========================================================

eligible_blocks = [
    x
    for x in primed_blocks
    if x["text"].strip()
]


for block in rng.sample(
    eligible_blocks,
    10,
):

    exact = block["text"]

    variant_1 = (
        exact.replace(
            "\t",
            "    ",
            1,
        )
        if "\t" in exact
        else exact + " "
    )

    variant_2 = (
        exact.replace(
            " := ",
            " = ",
            1,
        )
        if " := " in exact
        else exact.replace(
            " {",
            "{",
            1,
        )
    )

    variant_3 = (
        exact.replace(
            ")",
            ") ",
            1,
        )
        if ")" in exact
        else exact + "\n"
    )

    options = [
        exact,
        variant_1,
        variant_2,
        variant_3,
    ]

    rng.shuffle(options)

    add(
        "recognition",
        "verbatim",
        (
            "Which option exactly matches "
            f"lines {block['start_line']}-"
            f"{block['end_line']} of "
            f"`{block['file']}`?"
        ),
        options,
        exact,
    )


# =========================================================
# CUED TOPOLOGY RECALL — 10
# =========================================================

cued_symbols = [
    x
    for x in brain["symbol_memory"]
    if x["previous_symbol"]
]


for item in rng.sample(
    cued_symbols,
    10,
):

    add(
        "cued",
        "topology",
        (
            f"In `{item['file']}`, "
            "the symbol immediately after "
            f"`{item['previous_symbol']}` "
            "is what?"
        ),
        None,
        item["symbol"],
    )


# =========================================================
# CUED VERBATIM RECALL — 5
# =========================================================

multi_line = [
    x
    for x in primed_blocks
    if "\n" in x["text"]
]


for block in rng.sample(
    multi_line,
    5,
):

    lines = block["text"].split("\n")

    first = lines[0]

    remaining = "\n".join(
        lines[1:]
    )

    add(
        "cued",
        "verbatim",
        (
            "Given this exact first line:\n\n"
            f"{first}\n\n"
            f"from `{block['file']}` "
            f"lines {block['start_line']}-"
            f"{block['end_line']}, "
            "reproduce all remaining lines "
            "exactly."
        ),
        None,
        remaining,
    )


# =========================================================
# SAVE PRIVATE KEY
# =========================================================

os.makedirs(
    ".audit/recall-diagnostic",
    exist_ok=True,
)

key_path = (
    ".audit/recall-diagnostic/key.json"
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

public = {
    "schema_version": 1,

    "source_commit":
        brain["source_commit"],

    "question_count":
        len(questions),

    "note": (
        "Closed-book recognition/cued "
        "diagnostic after Cohort-1 priming. "
        "This is not a free-recall score."
    ),

    "questions":
        questions,
}


with open(
    "docs/RECALL_DIAGNOSTIC_EXAM.json",
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
    "DIAGNOSTIC QUESTIONS:",
    len(questions),
)

print(
    "PRIVATE KEY SHA256:",
    key_sha,
)

