# Holdout 2 Targeted Rehearsal

Holdout 2 is already frozen.

Nothing in this training phase may modify or retroactively improve
the frozen Holdout-2 score.

Only observed weak areas are rehearsed.

## Conceptual

Read:

docs/holdout2-targeted-rehearsal/conceptual-fields.json

For every card, explicitly learn and regenerate:

- package
- previous_symbol
- next_symbol
- calls
- db_reads
- db_writes
- routes
- side_effects

The goal is structured generative recall rather than approximate prose.

## Topology

Read:

docs/holdout2-targeted-rehearsal/topology-failures.json

Train exact:

- ordered symbol lists
- route handlers

Preserve order and exact symbol names.

## Verbatim

Read:

docs/holdout2-targeted-rehearsal/verbatim-failures.json

Train exact:

- indentation
- punctuation
- identifiers
- underscores
- backticks
- quotes
- line order

## Active-recall cycle

For every card:

CORRECTION
→ CLOSE
→ REGENERATE FROM MEMORY
→ COMPARE ONCE
→ CLOSE
→ REGENERATE MISSES ONCE

Do not repeatedly copy the correction.

## Measurement rule

Do NOT rerun Holdout 2.

Holdout 2 remains frozen historical evidence.

After this targeted rehearsal, measurement must use a new unseen
holdout generated with a different deterministic selection.
