#!/usr/bin/env python3

import json
import hashlib
import os

ROOT = os.path.abspath(
    os.path.join(
        os.path.dirname(__file__),
        "..",
    )
)

os.chdir(ROOT)

SOURCE = (
    "docs/"
    "UNAIDED_SOURCE_MEMORY_BASELINE_EXAM.json"
)

OUTPUT = (
    "docs/"
    "UNAIDED_SOURCE_MEMORY_SEALED_PROMPT.txt"
)

HASH_FILE = (
    "docs/"
    "UNAIDED_SOURCE_MEMORY_SEALED_PROMPT.sha256"
)


with open(
    SOURCE,
    encoding="utf-8",
) as f:
    exam = json.load(f)


payload = json.dumps(
    exam,
    ensure_ascii=False,
    indent=2,
) + "\n"


public_sha = hashlib.sha256(
    payload.encode("utf-8")
).hexdigest()


header = f"""UNAIDED SOURCE MEMORY EXAM — SEALED DELIVERY

STRICT RULES

1. From the moment this prompt is delivered until the answer file
   is frozen, use ZERO external retrieval.

2. Do NOT use:
   - tools
   - web
   - server
   - repository
   - Project Brain
   - source files
   - GitHub
   - connectors
   - search
   - previous chats
   - files
   - source snapshots
   - answer keys

3. The questions contained in this prompt are the ONLY exam
   material you may inspect.

4. Answer only from unaided active/model recall.

5. If you genuinely do not remember an answer, use null.
   Do not research or verify it.

6. Return exactly one JSON object:

   {{
     "answers": {{
       "1": ...,
       "2": ...,
       ...
       "80": ...
     }}
   }}

7. Do not explain your answers.

8. Do not verify your answers.

9. Freeze the JSON before any grading or retrieval occurs.

10. This test measures MEMORY, not retrieval capability.

PUBLIC_EXAM_SHA256={public_sha}

================ EXAM START ================

"""


with open(
    OUTPUT,
    "w",
    encoding="utf-8",
) as f:

    f.write(header)
    f.write(payload)


with open(
    HASH_FILE,
    "w",
    encoding="utf-8",
) as f:

    f.write(
        public_sha
        + "  "
        + "UNAIDED_SOURCE_MEMORY_BASELINE_EXAM.json\n"
    )


print(
    "SEALED EXAM GENERATED"
)

print(
    "questions:",
    exam["question_count"],
)

print(
    "public_exam_sha256:",
    public_sha,
)

print(
    "output:",
    OUTPUT,
)
