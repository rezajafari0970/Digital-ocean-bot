# Fresh-Chat Mastery Protocol

Purpose: make a new ChatGPT session recover the project's tested knowledge state before it makes implementation, architecture, line-level, database, runtime, or production claims.

## Phase 0 — Identity
1. Canonical repo: `/root/projects/Digital-ocean-bot-canonical-e2e`.
2. Branch: `checkpoint/final-e2e-20260929`.
3. Read `docs/NEW_CHAT_ENTRYPOINT.md`, `HANDOFF.md`, `PROJECT_STATE.md`, and `docs/SESSION_HANDOFF_BUNDLE.md`.
4. Never assume GitHub `main` is the production/canonical development state.

## Phase 1 — Mandatory gates
Run, in order:
- `python3 tools/knowledge-drift-gate.py`
- `python3 tools/handoff-acceptance-100.py`
- `python3 tools/deep-mastery-exam.py`
- `python3 tools/validate-blind-knowledge-challenge.py`

A fresh session may claim current project mastery only if drift is PASS, handoff is 100/100 READY, deep mastery is 100%, and blind challenge structure validates.

## Phase 2 — Knowledge hierarchy
For intent/why: `DEEP_REQUIREMENTS_BRAIN` → `INCIDENT_REGRESSION_BRAIN`.
For holistic feature/subsystem understanding: `FEATURE_OWNERSHIP_BRAIN` → `SUBSYSTEM_EXPLANATION_BRAIN` → `IMPACT_GRAPH`/`TRACEABILITY_MATRIX`.
For implementation: `FUNCTION_BEHAVIOR_BRAIN` → `SEMANTIC_KNOWLEDGE` → exact source snapshot.
For DB: `COLUMN_BEHAVIOR_BRAIN` → `SCHEMA_INDEX` → `LIVE_DB_BRAIN` → exact migration/source.
For lifecycle: `STATE_TRANSITION_BRAIN` → exact owner source.
For frontend: `FRONTEND_ACTION_STATE_BRAIN` → exact `web/static` source.
For tests: `TEST_BEHAVIOR_BRAIN` → exact test source.
For line questions: `project-brain-query.py` / `source-lookup.py`; never guess.
For known absences: `DEEP_EVIDENCE_INVESTIGATION` and `MASTER_KNOWLEDGE_SCORE_V3`.

## Phase 3 — Claim discipline
- Bind exact line claims to the snapshot/source commit.
- Treat lexical calls/callers/ownership matches as navigation evidence, not compiler/runtime proof.
- Treat known absence as knowledge, not as product coverage.
- Before a production revision claim, run `python3 tools/verify-production-revision.py`.
- If product/source paths changed after a snapshot, regenerate knowledge before using it as current.
- Preserve `.audit`; never delete/reset it blindly.

## Phase 4 — New-chat self-test
The new session should answer a sample spanning intent, cross-package ownership, route→handler, DB column→migration→reader/writer, state→owner, UI→API, test→assertion, incident→guardrail, exact line retrieval, and production revision verification. Answers must cite/refer to exact project evidence internally rather than rely on memory.

## Meaning of 100%
`MASTER_KNOWLEDGE_SCORE_V3 = 100%` means epistemic completeness for the defined current project knowledge: facts are directly evidenced, explained, or confirmed absent. It does not mean unaided memorization of every source token and it does not claim missing product tests/history/access exist.

## Phase 5 — Genuine fresh-session exam
Use `docs/FRESH_SESSION_EXAM_PACK.json`, which contains questions but no answers. The fresh session must retrieve evidence and freeze an `ANSWERS.json` before `tools/grade-fresh-session-exam.py ANSWERS.json` is run. Do not expose/read `docs/FRESH_SESSION_EXAM_KEY.json` while answering. This is the operational test that the new chat can consume the knowledge system, not merely that the knowledge files exist.
