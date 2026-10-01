# Session Bootstrap Protocol

A new ChatGPT session must perform this sequence before changing production code:

1. Read `docs/CHATGPT_MASTER_CONTEXT.md`, `PROJECT_CONTEXT.md`, `PROJECT_STATE.md`, `HANDOFF.md`, and `docs/CODEBASE_MAP.md`.
2. Run `git status --short --branch`, `git rev-parse HEAD`, and recent `git log` in the canonical repository.
3. Compare local branch with its remote; do not silently switch to `main`.
4. Check API/worker service state and, when relevant, deployed binary/source revision.
5. Inspect the files named by HANDOFF and recent commits before editing.
6. Inspect live migration level before schema-dependent work.
7. Preserve untracked `.audit` evidence unless deliberately archived/committed/removed after review.
8. Implement/test the exact current boundary, not a reconstructed older plan.
9. After a successful logical stage: update PROJECT_STATE/HANDOFF, commit, push, and record deploy/test outcome.

The continuity documents summarize intent; Git source and production runtime remain authoritative for exact implementation details. When they disagree, investigate and update the docs rather than forcing runtime to match stale prose.

10. Use `docs/FEATURE_FLOW_INDEX.md` to locate the vertical feature path before broad searching.
11. Follow `docs/CHANGE_PROTOCOL.md` for implementation, verification, deploy and checkpoint discipline.
12. Read `docs/HISTORICAL_DECISION_LEDGER.md` before architectural changes so rejected patterns and prior incidents are not reintroduced.
13. Use `docs/REQUIREMENTS_MATRIX.md` as an invariant checklist when changing a subsystem.
14. Read `docs/PROJECT_MANIFEST.json` first for machine-readable navigation, then run `tools/check-project-continuity.sh` to detect branch/HEAD/runtime drift before relying on the manifest snapshot.
15. At the end of a completed logical stage, run `tools/checkpoint-project.sh "<commit message>"` after updating HANDOFF/PROJECT_STATE. It regenerates the machine-readable manifest/current snapshot, validates JSON, commits and pushes the continuity checkpoint.
