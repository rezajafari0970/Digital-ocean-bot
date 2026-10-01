# Distributed Source-Wide Generative Rehearsal

## Purpose

Previous targeted rehearsal improved memorized items but did not
generalize sufficiently to unseen Holdout-4 material.

This phase therefore stops training against holdout failures.

The training domain is now the source knowledge corpus itself.

## Conceptual

Cover every entry in:

SOURCE_INTERNALIZATION_BRAIN.symbol_memory

For each card learn the complete structured tuple:

symbol
file
package
previous_symbol
next_symbol
calls
db_reads
db_writes
routes
side_effects

Training cycle:

READ
→ CLOSE
→ GENERATE
→ COMPARE ONCE
→ CLOSE
→ REGENERATE MISSES ONCE

## Topology

Cover:

- every indexed file
- complete ordered symbol list for every file
- every indexed route
- exact handler for every route

Partial symbol lists are failures.

Ordering is part of the answer.

## Verbatim

Previous exact-source training covered the first 5 Verbatim packs.

This phase expands training to the first 10 Verbatim packs.

Block boundaries must remain unchanged.

Exact recall includes:

tabs
spaces
newlines
backticks
quotes
underscores
punctuation
identifiers
capitalization
line order

## Checkpointing

Process one manifest unit at a time.

After a complete unit emit:

DISTRIBUTED UNIT N COMPLETE

If a unit truncates or fails, restart only that unit.

Never mark a partial unit complete.

## Measurement integrity

Do not use Holdout 2, 3, or 4 as training material.

Do not open any future holdout.

Do not open `.audit`.

After all distributed units complete, enter CLOSED-BOOK MODE.

The next measurement must use a new unseen holdout.

Conceptual and Topology may be described as source-wide only because
their indexed source domains are covered here.

Verbatim remains partial and must always report its actual pack/source
coverage rather than claiming whole-repository exact memory.
