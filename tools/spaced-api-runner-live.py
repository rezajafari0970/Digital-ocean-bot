#!/usr/bin/env python3

import json
import os
import subprocess
import tempfile
import urllib.request
import urllib.error
from datetime import datetime, timezone

ROOT = os.path.dirname(
    os.path.dirname(os.path.abspath(__file__))
)

STATE = os.path.join(
    ROOT,
    ".local/spaced-rehearsal-progress.json",
)

RESULT_DIR = os.path.join(
    ROOT,
    ".local/spaced-results",
)

os.makedirs(RESULT_DIR, exist_ok=True)


def call_model(prompt):

    key = os.environ.get("OPENAI_API_KEY")

    if not key:
        raise RuntimeError("OPENAI_API_KEY missing")

    payload = {
        "model": os.environ.get(
            "OPENAI_MODEL",
            "gpt-5.6",
        ),
        "input": prompt,
        "max_output_tokens": 2500,
    }

    req = urllib.request.Request(
        "https://api.openai.com/v1/responses",
        json.dumps(payload).encode(),
        {
            "Authorization": "Bearer " + key,
            "Content-Type": "application/json",
        },
    )

    try:
        with urllib.request.urlopen(
            req,
            timeout=180,
        ) as response:

            data = json.load(response)

    except urllib.error.HTTPError as e:

        raw = e.read().decode(
            "utf-8",
            "replace",
        )

        try:
            obj = json.loads(raw)
            err = obj.get("error") or {}
            code = err.get("code")
        except Exception:
            code = None

        if code == "credit_balance_exhausted":
            raise SystemExit(75)

        if e.code == 429 or e.code >= 500:
            raise SystemExit(76)

        raise RuntimeError(
            f"API HTTP {e.code}: {raw[:500]}"
        )

    if data.get("error"):
        raise RuntimeError(
            str(data["error"])
        )

    texts = []

    for output in data.get("output", []):
        for content in output.get("content", []):

            ctype = content.get("type")

            if ctype == "output_text":
                texts.append(
                    content.get("text", "")
                )

            if ctype in (
                "refusal",
                "safety",
            ):
                raise SystemExit(77)

    return (
        "".join(texts).strip(),
        data.get("id"),
    )


def parse_json(text):

    try:
        return json.loads(text)
    except Exception:
        return None


def cue_prompt(card):

    kind = card["kind"]
    cue = card["cue"]

    base = (
        "Closed-book retrieval practice. "
        "Use only your existing memory. "
        "Do not use tools or external sources. "
        "Return only valid JSON. "
    )

    if kind == "conceptual":

        contract = (
            "Return exactly these fields: "
            "package, previous_symbol, next_symbol, "
            "calls, db_reads, db_writes, routes, "
            "side_effects."
        )

    elif kind == "topology":

        if "file" in cue:
            contract = (
                'Return {"symbols":[...]} containing '
                "the complete ordered symbols."
            )
        else:
            contract = (
                'Return {"handler":"..."} containing '
                "the exact handler."
            )

    else:

        contract = (
            'Return {"text":"..."} containing '
            "the exact recalled source block."
        )

    return (
        base
        + contract
        + "\nCUE:\n"
        + json.dumps(
            cue,
            ensure_ascii=False,
        )
    )


def correction_prompt(card):

    return (
        "Study this correction once. "
        "Do not use tools or external sources. "
        "Memorize the exact answer for the cue.\n\n"
        "CUE:\n"
        + json.dumps(
            card["cue"],
            ensure_ascii=False,
        )
        + "\n\nCORRECT ANSWER:\n"
        + json.dumps(
            card["answer"],
            ensure_ascii=False,
        )
        + "\n\nReply only with: CORRECTION_SEEN"
    )


def regenerate_prompt(card):

    return (
        cue_prompt(card)
        + "\n\nThis is a regeneration attempt "
        "after the correction has been closed. "
        "Produce the answer again from memory."
    )


def atomic_json(path, obj):

    os.makedirs(
        os.path.dirname(path),
        exist_ok=True,
    )

    fd, tmp = tempfile.mkstemp(
        dir=os.path.dirname(path),
        prefix=".tmp-",
        text=True,
    )

    try:
        with os.fdopen(
            fd,
            "w",
            encoding="utf-8",
        ) as f:

            json.dump(
                obj,
                f,
                ensure_ascii=False,
                indent=2,
            )

            f.write("\n")
            f.flush()
            os.fsync(f.fileno())

        os.replace(tmp, path)

    finally:
        if os.path.exists(tmp):
            os.unlink(tmp)


def progress(*args):

    return subprocess.run(
        [
            "python3",
            os.path.join(
                ROOT,
                "tools/spaced-progress.py",
            ),
            *args,
        ],
        cwd=ROOT,
        check=True,
        capture_output=True,
        text=True,
    )


def main():

    with open(
        STATE,
        encoding="utf-8",
    ) as f:
        state = json.load(f)

    cursor = state["cursor"]

    if cursor is None:
        print("TRAINING_COMPLETE")
        return

    path = os.path.join(
        ROOT,
        cursor["path"],
    )

    with open(
        path,
        encoding="utf-8",
    ) as f:
        cards = json.load(f)

    result = {
        "cursor": cursor,
        "started_at":
            datetime.now(
                timezone.utc
            ).isoformat(),
        "cards": [],
    }

    try:

        for number, card in enumerate(
            cards,
            1,
        ):

            blind_text, blind_id = call_model(
                cue_prompt(card)
            )

            blind = parse_json(
                blind_text
            )

            first_exact = (
                blind == card["answer"]
            )

            item = {
                "card": number,
                "kind": card["kind"],
                "blind_response_id": blind_id,
                "first_valid_json":
                    blind is not None,
                "first_exact":
                    first_exact,
                "regenerated":
                    False,
                "regenerated_exact":
                    None,
            }

            if not first_exact:

                correction_text, correction_id = (
                    call_model(
                        correction_prompt(card)
                    )
                )

                item["correction_response_id"] = (
                    correction_id
                )

                item["correction_ack"] = (
                    correction_text.strip()
                    == "CORRECTION_SEEN"
                )

                regen_text, regen_id = call_model(
                    regenerate_prompt(card)
                )

                regen = parse_json(
                    regen_text
                )

                item["regenerated"] = True
                item["regen_response_id"] = regen_id
                item["regen_valid_json"] = (
                    regen is not None
                )
                item["regenerated_exact"] = (
                    regen == card["answer"]
                )

            result["cards"].append(item)

    except SystemExit as e:

        code = int(e.code)

        result["finished_at"] = (
            datetime.now(
                timezone.utc
            ).isoformat()
        )

        if code == 75:
            result["status"] = "CREDIT_EXHAUSTED"
            atomic_json(
                os.path.join(
                    RESULT_DIR,
                    "last-paused.json",
                ),
                result,
            )
            print("CREDIT_EXHAUSTED")
            raise

        if code == 76:
            result["status"] = "RETRYABLE"

            atomic_json(
                os.path.join(
                    RESULT_DIR,
                    "last-retryable.json",
                ),
                result,
            )

            progress(
                "mark",
                "RETRYABLE",
            )

            print("RETRYABLE")
            raise

        if code == 77:
            result["status"] = "SAFETY_BLOCKED"

            atomic_json(
                os.path.join(
                    RESULT_DIR,
                    "last-safety-blocked.json",
                ),
                result,
            )

            progress(
                "mark",
                "SAFETY_BLOCKED",
            )

            print("SAFETY_BLOCKED")
            return

        raise

    result["finished_at"] = (
        datetime.now(
            timezone.utc
        ).isoformat()
    )

    result["status"] = "COMPLETE"

    result["first_exact_count"] = sum(
        1
        for x in result["cards"]
        if x["first_exact"]
    )

    result["regenerated_exact_count"] = sum(
        1
        for x in result["cards"]
        if x.get("regenerated_exact") is True
    )

    ident = (
        f"pass-{cursor['pass']}"
        f"-unit-{cursor['unit']:03d}"
        f"-micro-{cursor['micro']:02d}"
    )

    atomic_json(
        os.path.join(
            RESULT_DIR,
            ident + ".json",
        ),
        result,
    )

    # COMPLETE means the entire rehearsal cycle
    # was completed, not that blind recall was perfect.
    progress(
        "mark",
        "COMPLETE",
    )

    print(
        json.dumps(
            {
                "status": "COMPLETE",
                "cursor_completed": cursor,
                "first_exact":
                    result[
                        "first_exact_count"
                    ],
                "cards":
                    len(cards),
                "regenerated_exact":
                    result[
                        "regenerated_exact_count"
                    ],
            },
            ensure_ascii=False,
        )
    )

    print(
        progress(
            "next"
        ).stdout.strip()
    )


if __name__ == "__main__":
    main()
