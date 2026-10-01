# Resumable Active-Rehearsal Orchestrator

This protocol replaces the monolithic 3,125-question execution.

The purpose is to prevent tool-output truncation from destroying
training progress.

## Core rule

Process exactly ONE rehearsal unit at a time.

The chat itself performs:

PACK
→ ACTIVE RECALL
→ CORRECTION
→ REGENERATE MISSES

Large answers must never be printed through terminal/tool output.

Tool output should contain only metadata required to locate the
three files belonging to the current unit.

## Unit order

Use:

`docs/ACTIVE_RECALL_REHEARSAL_MANIFEST.json`

There are exactly 26 units.

Required order:

- Units 01–08: Topology
- Units 09–21: Conceptual
- Units 22–26: Verbatim

Never skip or reorder units.

## Processing one unit

For unit N:

### Phase A — Pack

1. Determine only the metadata for unit N.
2. Read the unit's original `pack`.
3. Internalize it completely.
4. Stop looking at the pack.

### Phase B — Recall

5. Read the unit's `recall` file.
6. Answer every recall prompt from memory.
7. Do not reopen pack.
8. Do not open correction yet.

The recall answers belong in the model/chat context.

Do not dump all answers through shell/tool output.

### Phase C — Correction

9. Only after recall is complete, read the unit's `correction`.
10. Compare recall against correction.
11. Identify every incorrect or incomplete item.
12. Internalize the corrected values.

### Phase D — Regeneration

13. Stop looking at pack and correction.
14. Regenerate all missed items once from memory.
15. Do not verify them again.

### Phase E — Checkpoint

Only after all four phases completed successfully emit:

UNIT NN/26 COMPLETE — <kind>

Example:

UNIT 03/26 COMPLETE — topology

That line is the only completion checkpoint.

## Failure rule

If:

- a tool call fails
- output truncates
- a file cannot be completely read
- the model cannot establish that Recall finished before Correction
- regeneration did not complete

then the current unit is NOT complete.

Restart that same unit from its original pack.

Never continue to the next unit after an uncertain or partial unit.

## Forbidden material

Never open:

- REHEARSAL_HOLDOUT_2_EXAM.json
- any `.audit` path
- any answer key
- any grader
- any other holdout exam

Do not modify project Source.

Do not commit rehearsal answers.

## Completion

After:

UNIT 26/26 COMPLETE — verbatim

immediately stop all:

- tools
- server
- repository
- files
- Project Brain
- Source Snapshot
- GitHub
- web
- search
- connectors
- shell
- Python
- lookup

Then emit exactly:

REHEARSAL COMPLETE — CLOSED-BOOK READY

No further tool call is permitted after that line.
