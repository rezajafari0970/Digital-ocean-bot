# Spaced Multi-Pass Source Rehearsal

## Objective

Convert source familiarity into stable generative recall.

One-pass exposure is no longer considered sufficient.

## Passes

Run:

PASS 1 — source-wide conceptual/topology
PASS 2 — source-wide conceptual/topology
PASS 3 — source-wide conceptual/topology

and:

VERBATIM PASS 1
VERBATIM PASS 2
VERBATIM PASS 3

Each pass uses a different deterministic shuffle.

## Card cycle

For every card:

CUE ONLY
→ GENERATE ANSWER BEFORE REVEAL
→ REVEAL ANSWER ONCE
→ EXACT COMPARE
→ CLOSE ANSWER
→ REGENERATE MISSES

Do not read answer before attempting recall.

## Failure retention

If a card fails exact recall:

it MUST remain active for the next pass.

Do not retire it.

## Retirement rule

A card may be treated as stable only after:

TWO CONSECUTIVE EXACT RECALLS

without looking at the answer first.

One successful recall is insufficient.

## Conceptual

Exact object:

package
previous_symbol
next_symbol
calls
db_reads
db_writes
routes
side_effects

All fields matter.

## Topology

File cards require:

complete ordered symbols[]

Route cards require:

exact handler

Partial arrays fail.

## Verbatim

Cue:

file
line range
first line

Recall:

complete exact block

Exact means:

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

After every completed unit emit:

SPACED PASS <P> UNIT <N> COMPLETE

If interrupted:

do not mark current unit complete.

Resume that unit from its beginning.

## Forbidden

Never open:

Holdout 2
Holdout 3
Holdout 4
Holdout 5
future holdouts
.audit
answer keys
graders

## Completion

After all six passes are complete:

stop all tools and lookups.

Enter CLOSED-BOOK MODE.

Emit exactly:

SPACED REHEARSAL COMPLETE — CLOSED-BOOK READY
