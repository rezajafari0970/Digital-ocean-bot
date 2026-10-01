# New Chat Entrypoint

For a fresh ChatGPT session continuing this project, do not reconstruct from chat history and do not start from GitHub main.

1. Canonical repository: `/root/projects/Digital-ocean-bot-canonical-e2e` on `serverprojects.ptr.network`.
2. Read `docs/SESSION_HANDOFF_BUNDLE.md`, then `HANDOFF.md` and `PROJECT_STATE.md`.
3. Run `tools/check-project-continuity.sh` and `python3 tools/handoff-acceptance-100.py`.
4. Use `tools/project-brain-query.py` for exact file:line, symbol, endpoint, table, path or text retrieval.
5. Use `FULL_SOURCE_KNOWLEDGE.json` for exact commit-pinned source lines; never guess line contents.
6. Before behavior changes, identify requirement IDs in `INTENT_REQUIREMENTS_BRAIN.json` and inspect `IMPACT_GRAPH.json` / `TRACEABILITY_MATRIX.json`.
7. Before production claims, run `tools/verify-production-revision.py`; Git HEAD and production may legitimately differ until deployment.
8. Preserve `.audit` evidence and do not reset/delete it blindly.
9. Continue from the exact boundary in `HANDOFF.md`; inspect live source/runtime before mutation.

Acceptance policy: `docs/HANDOFF_ACCEPTANCE_100.json` must report `ready: true` and score `100` before this handoff is called READY. This means tested recoverability/traceability, not unaided memorization of every token.

## Mastery bootstrap
Read `docs/FRESH_CHAT_MASTERY_PROTOCOL.md` and run `python3 tools/fresh-chat-mastery-gate.py` before claiming full current-project mastery. Require `FRESH CHAT MASTERY READY`.

## Independent fresh-session proof
When explicitly asked to prove fresh-session mastery, follow `docs/FRESH_SESSION_EXAM_INSTRUCTIONS.md`. Freeze all 40 answers before any grading and never inspect `.audit/fresh-session-exam/` or historical/deleted answer-key material.

## Independent verification status

`docs/FRESH_SESSION_MASTERY_PROOF.json` records the independently executed Fresh Session Exam v3 proof.

Treat it as evidence of sampled cross-layer recoverability only when `verified: true`.

The recorded proof is **40/40 across all eight exam categories**. Source/runtime drift gates must still be run after future product changes.
