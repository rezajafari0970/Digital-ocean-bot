# Persistent Spaced-Rehearsal Ledger

`tools/spaced-progress.py` is the authoritative cursor/checkpoint ledger. The state file is local runtime state and is gitignored.

At the beginning of every continuation run `python3 tools/spaced-progress.py next` and process only the returned cursor micro. After the full micro cycle, atomically mark it `COMPLETE`. On ordinary technical interruption mark `RETRYABLE`; do not advance. On an actual OpenAI/tool safety decision mark `SAFETY_BLOCKED`; never inspect, transform, encode, paraphrase, split further, or retry that blocked content; advance to the next cursor.

Never infer COMPLETE from skipped ranges. Existing historical completion through Pass 1 Unit 23 is seeded. Explicit safety blocks may be recorded from conversation evidence. The ledger does not make ChatGPT run in the background; a new active turn is still required after a turn ends.
