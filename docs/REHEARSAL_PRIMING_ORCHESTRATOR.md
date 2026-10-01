# Rehearsal Priming Orchestrator — Cohort 1

Run this protocol in a fresh chat.

During training, server/file access is allowed only for the explicitly
named training files.

Never open:

- holdout exams
- `.audit`
- answer keys
- graders

## Training order

Read:

`docs/ACTIVE_RECALL_REHEARSAL_MANIFEST.json`

Process units in this order:

1. all Topology units
2. all Conceptual units
3. all Verbatim units

For every unit:

1. Read the original `pack` completely.

2. Stop looking at the original pack.

3. Read the matching `recall` file.

4. Answer every recall question from memory.

5. Commit those answers in the conversation before viewing correction.

6. Only then read the matching `correction` file.

7. Compare the generated answers with correction.

8. Identify misses.

9. Close both the original pack and correction material.

10. Regenerate every missed answer once from memory.

11. Continue to the next unit.

Do not save rehearsal answers into the repository.

The purpose is active-context learning rather than project mutation.

## End of training

After all 26 units are complete, stop all:

- tools
- server access
- file access
- repository access
- Project Brain access
- web/search
- connectors
- source lookup

Then emit exactly:

REHEARSAL COMPLETE — CLOSED-BOOK READY

From that moment onward, only the holdout exam supplied directly by
the user may be read.

No other lookup is permitted until holdout answers are frozen.
