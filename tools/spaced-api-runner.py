#!/usr/bin/env python3

import argparse
import json
import os
import urllib.request
import urllib.error

ROOT = os.path.dirname(
    os.path.dirname(
        os.path.abspath(__file__)
    )
)

STATE = os.path.join(
    ROOT,
    ".local/spaced-rehearsal-progress.json",
)


def call_model(prompt):
    key = os.environ.get("OPENAI_API_KEY")

    if not key:
        raise RuntimeError(
            "OPENAI_API_KEY missing"
        )

    payload = {
        "model": os.environ.get(
            "OPENAI_MODEL",
            "gpt-5.6",
        ),
        "input": prompt,
        "max_output_tokens": 2000,
    }

    req = urllib.request.Request(
        "https://api.openai.com/v1/responses",
        json.dumps(payload).encode(),
        {
            "Authorization":
                "Bearer " + key,

            "Content-Type":
                "application/json",
        },
    )

    try:
        with urllib.request.urlopen(
            req,
            timeout=120,
        ) as response:

            data = json.load(response)

    except urllib.error.HTTPError as e:

        raw = e.read().decode(
            "utf-8",
            "replace",
        )

        raise RuntimeError(
            f"HTTP {e.code}: {raw[:1000]}"
        )

    texts = []

    for output in data.get(
        "output",
        [],
    ):
        for content in output.get(
            "content",
            [],
        ):
            if (
                content.get("type")
                == "output_text"
            ):
                texts.append(
                    content.get(
                        "text",
                        "",
                    )
                )

    return (
        "".join(texts).strip(),
        data.get("id"),
    )


def build_prompt(card):

    kind = card["kind"]
    cue = card["cue"]

    prefix = (
        "CLOSED-BOOK retrieval practice. "
        "Do not use tools or external sources. "
        "Return ONLY valid JSON. "
    )

    if kind == "conceptual":

        contract = (
            "Return an object with exactly "
            "these keys: package, "
            "previous_symbol, next_symbol, "
            "calls, db_reads, db_writes, "
            "routes, side_effects."
        )

    elif kind == "topology":

        if "file" in cue:
            contract = (
                'Return {"symbols":[...]} '
                "with the complete ordered "
                "symbol list."
            )
        else:
            contract = (
                'Return {"handler":"..."} '
                "with the exact handler."
            )

    else:

        contract = (
            'Return {"text":"..."} '
            "containing the exact recalled "
            "source block."
        )

    return (
        prefix
        + contract
        + "\nCUE:\n"
        + json.dumps(
            cue,
            ensure_ascii=False,
        )
    )


def main():

    parser = argparse.ArgumentParser()

    parser.add_argument(
        "--dry-run",
        action="store_true",
        required=True,
    )

    parser.parse_args()

    with open(
        STATE,
        encoding="utf-8",
    ) as f:
        state = json.load(f)

    cursor = state["cursor"]

    if not cursor:
        raise SystemExit(
            "No active cursor"
        )

    card_path = os.path.join(
        ROOT,
        cursor["path"],
    )

    with open(
        card_path,
        encoding="utf-8",
    ) as f:
        cards = json.load(f)

    results = []

    for number, card in enumerate(
        cards,
        1,
    ):

        prompt = build_prompt(card)

        text, response_id = call_model(
            prompt
        )

        try:
            generated = json.loads(text)
            valid_json = True

        except Exception:
            generated = None
            valid_json = False

        exact = (
            valid_json
            and generated == card["answer"]
        )

        results.append({
            "card":
                number,

            "kind":
                card["kind"],

            "response_id":
                response_id,

            "valid_json":
                valid_json,

            "exact":
                exact,
        })

    print(
        json.dumps(
            {
                "mode":
                    "DRY_RUN",

                "ledger_mutated":
                    False,

                "cursor":
                    cursor,

                "results":
                    results,

                "all_exact":
                    all(
                        r["exact"]
                        for r in results
                    ),
            },
            ensure_ascii=False,
            indent=2,
        )
    )


if __name__ == "__main__":
    main()
