# Conceptual Micro-Rehearsal Protocol

Units 09–21 retain their original unit identity, but each is split into micro-units of at most 20 conceptual symbols/questions to prevent tool truncation.

For a parent Unit NN: process micro 01..K in manifest order. Each micro independently runs PACK → ACTIVE RECALL → CORRECTION → REGENERATE MISSES → micro checkpoint. If a micro fails/truncates, restart only that micro from its PACK. Do not repeat completed micros.

A parent `UNIT NN/26 COMPLETE — conceptual` checkpoint is valid only after every micro-unit belonging to NN has a valid completion checkpoint. Micro completion does not advance the global unit number by itself.

Use `python3 tools/conceptual-micro-info.py UNIT MICRO` for compact metadata. Never open Holdout, `.audit`, key or grader. Units 01–08 remain complete from the existing chat checkpoint; this protocol changes only conceptual Units 09–21.
