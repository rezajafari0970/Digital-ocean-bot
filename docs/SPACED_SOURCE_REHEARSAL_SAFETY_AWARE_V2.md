# Safety-Aware Micro-Resume v2

This protocol never bypasses or retries around a safety decision by changing, splitting, encoding, paraphrasing, or otherwise transforming blocked content.

## Statuses

Each micro ends in exactly one status:

- `COMPLETE`: full CUE → GENERATE BEFORE REVEAL → REVEAL → EXACT COMPARE → CLOSE → REGENERATE MISSES completed.
- `RETRYABLE`: ordinary technical interruption (timeout, connection loss, truncation). Do not advance; retry the same micro later.
- `SAFETY_BLOCKED`: a safety decision prevents processing the micro. Do not inspect, transform, paraphrase, split further, encode, or retry the blocked content. Record only pass/unit/micro identity and advance to the next micro.

`SAFETY_BLOCKED` is never COMPLETE, never counts as learned, and never qualifies a parent unit for full-coverage claims.

## Checkpoints

For COMPLETE:
`SPACED PASS P UNIT U MICRO M COMPLETE`

For RETRYABLE:
`RESUME FROM SPACED PASS P UNIT U MICRO M`

For safety block:
`SPACED PASS P UNIT U MICRO M SAFETY_BLOCKED`

After SAFETY_BLOCKED, continue with the next manifest micro. Do not attempt the blocked micro again during this training run.

A parent unit with one or more blocked micros is reported as:
`SPACED PASS P UNIT U/T PARTIAL — SAFETY_BLOCKED=N`

A parent unit with all micros COMPLETE is reported normally:
`SPACED PASS P UNIT U/T COMPLETE`

## Final reporting

At the end of each pass report counts for COMPLETE, RETRYABLE, and SAFETY_BLOCKED micros. At final completion, report training coverage explicitly. Never claim 100% training coverage if any micro is SAFETY_BLOCKED.

Holdouts and `.audit` remain forbidden. Existing valid checkpoints remain valid. Current continuation point supplied by the conversation is Pass 1 / Unit 24 / Micro 01.
