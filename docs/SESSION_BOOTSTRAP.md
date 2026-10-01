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

16. Read `docs/SESSION_HANDOFF_BUNDLE.md` as the single fast-entry context bundle; use its sections as navigation, then verify exact details against source/runtime.
17. Run `python3 tools/audit-handoff-readiness.py` before a planned chat migration. Do not treat the handoff as self-sufficient if any readiness check fails.
18. Use `docs/SYMBOL_NAVIGATION.md` / `docs/SYMBOL_INDEX.json` for route→handler and symbol→file:line navigation before broad grep; regenerate with `python3 tools/build-symbol-index.py` after structural code changes.
19. Use `docs/SCHEMA_NAVIGATION.md` / `docs/SCHEMA_INDEX.json` for migration→table→code persistence navigation; regenerate with `python3 tools/build-schema-index.py` after migration/schema changes, and still verify the live DB migration level before mutation.
20. Use `docs/TEST_NAVIGATION.md` / `docs/TEST_MAP.json` to select focused verification for the subsystem being changed; regenerate with `python3 tools/build-test-map.py` after test/package structure changes.
21. Use `docs/RUNTIME_NAVIGATION.md` / `docs/RUNTIME_MAP.json` for source→build→binary→systemd→health navigation; regenerate with `python3 tools/build-runtime-map.py` when deployment/runtime structure changes. Do not infer production commit identity from an active service alone.
22. Run `python3 tools/verify-production-revision.py` before claiming production is on the current source revision. Require `verified: true`; service health alone is insufficient.
23. For exact file/line questions, use `docs/FULL_SOURCE_KNOWLEDGE.json` or `python3 tools/source-lookup.py FILE LINE [END]` before reopening source. Always bind line answers to `source_commit`; regenerate with `python3 tools/build-source-knowledge.py` after source changes.

24. Use `docs/SEMANTIC_KNOWLEDGE.json` as the semantic/navigation layer (roles, symbol ranges, candidate calls/callers, SQL tables, endpoint literals), then confirm exact behavior from the commit-pinned line snapshot. Regenerate after Go source changes.
25. Use `tools/project-brain-query.py` as the first unified retrieval interface for file:line, symbol, endpoint, table, path and exact-text questions; results are commit-pinned and should be verified against live source/runtime when freshness matters.
26. Before changing behavior, identify the governing requirement(s) in `docs/INTENT_REQUIREMENTS_BRAIN.json` and inspect `docs/IMPACT_GRAPH.json` for affected code/schema/test neighborhoods; preserve the rationale unless the user explicitly changes the requirement.
27. For UI changes, read `docs/FRONTEND_BRAIN.json`/`.md` first to map pages, API calls, actions and browser handlers before editing `web/static/*`.
28. For scheduler/lifecycle/provisioning changes, read `docs/LIFECYCLE_STATE_BRAIN.json`/`.md`, then verify exact transition ownership and persistence in source before mutation.
29. Before schema-sensitive work, regenerate/read `docs/LIVE_DB_BRAIN.json`; it contains production schema metadata only and must not contain application rows/secrets.
30. Before worker/scheduler/runtime cadence changes, read `docs/RUNTIME_WORKER_BRAIN.json`, then verify exact ownership/control flow in source and live services.
33. Read `docs/DEEP_REQUIREMENTS_BRAIN.md` before interpreting or changing subsystem behavior; it is the fine-grained intent/specification layer beyond the coarse requirement IDs.
34. For implementation-understanding questions, use `docs/FUNCTION_BEHAVIOR_BRAIN.json` to retrieve function ranges/signatures, routes, calls/callers, DB reads/writes, side-effect classes and error evidence before drawing behavioral conclusions.

35. For database-understanding questions, use `docs/COLUMN_BEHAVIOR_BRAIN.json` for live column metadata, migration origin and conservative function-level reader/writer evidence; confirm dynamic or indirect access in exact source when necessary.
36. For verification-understanding questions, use `docs/TEST_BEHAVIOR_BRAIN.json` to inspect exact test ranges, assertions, fixtures, candidate calls, subtests and endpoint/table evidence rather than inferring coverage from filenames alone.
37. For lifecycle/state questions, use `docs/STATE_TRANSITION_BRAIN.json` for owner functions, state-write evidence, guards, retry/timing evidence and DB touches; do not infer a from→to edge unless exact source supports it.
38. For frontend behavior questions, use `docs/FRONTEND_ACTION_STATE_BRAIN.json` for app.js function ranges, API literals, DOM selectors, class/storage mutations, action/event ownership, loading and error evidence before inspecting exact source.
39. Before modifying a subsystem, inspect `docs/INCIDENT_REGRESSION_BRAIN.json` for prior failure classes, durable lessons and anti-regression guardrails; related commit matches are navigation candidates and require source/history inspection before causal claims.
40. Use `docs/DEEP_MASTERY_EXAM.json` as the evidence-backed domain mastery scorecard. A domain reaches 100 only when all defined knowledge/traceability gates for that domain pass; this is distinct from unaided memorization of every token.
41. Use `docs/BLIND_KNOWLEDGE_CHALLENGE.json` for cross-layer mastery checks. Its questions require route→handler, DB→migration→functions, state→owner, UI→API, test→assertion, incident→guardrail, intent and exact-line composition; answers must be evidence-backed, not guessed.
42. Run `python3 tools/knowledge-drift-gate.py` before relying on Brain snapshots as current. Knowledge/docs-only commits may advance HEAD, but any change under cmd/internal/migrations/web/static/deploy makes older source knowledge stale until regeneration.
43. For holistic subsystem understanding, start with `docs/SUBSYSTEM_EXPLANATION_BRAIN.json`: it composes purpose, responsibilities, owned code, APIs, DB touches, dependencies, tests, failure evidence and incident guardrails before drilling into exact source.
