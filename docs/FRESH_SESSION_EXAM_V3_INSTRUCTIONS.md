# Fresh Session Exam v3 — New Chat Instructions

Use no previous-chat history. Do not open any `.audit` path, grader, deleted blob/reflog, or historical answer-key material.

1. Work in the canonical repository/branch and read `docs/KNOWLEDGE_RETRIEVAL_CONTRACT.md`.
2. Run `python3 tools/fresh-chat-mastery-gate.py` and require READY.
3. Read `docs/FRESH_SESSION_EXAM_V3.json`.
4. For every question containing `retrieval`, run exactly `python3 tools/project-knowledge-contract.py <type> '<query>'` and use the returned JSON object **unchanged** as that answer.
5. For History questions, retrieve the matching incident from `docs/INCIDENT_REGRESSION_BRAIN.json` and answer only `lesson` and `prevent`.
6. Freeze exactly 40 answers in `/tmp/fresh-session-v3-answers.json` as `{ "answers": { ... } }`.
7. Do not grade. Return only SHA256 of the frozen answer file.
