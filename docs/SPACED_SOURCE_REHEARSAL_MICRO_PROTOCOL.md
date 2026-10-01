# Spaced Rehearsal Micro-Resume Protocol

This preserves all original cards and pass order while reducing checkpoint granularity. Conceptual/topology micros contain at most 3 cards; verbatim micros at most 2.

Existing checkpoint `SPACED PASS 1 UNIT 12/168 COMPLETE` remains valid. Do not repeat parent Units 1–12. Resume at Pass 1, Unit 13, Micro 1.

Each micro independently performs CUE → GENERATE BEFORE REVEAL → REVEAL → EXACT COMPARE → CLOSE → REGENERATE MISSES. Checkpoint only after the entire micro completes. If interrupted or blocked, restart only that micro. A parent Unit is complete only after all its micros complete. Then continue to the next parent Unit.

Do not bypass safety/tool blocks. If a specific micro cannot be processed, stop on that micro and report its exact pass/unit/micro checkpoint; do not mark it complete or skip it.

Holdouts and `.audit` remain forbidden. Final completion semantics are unchanged.
