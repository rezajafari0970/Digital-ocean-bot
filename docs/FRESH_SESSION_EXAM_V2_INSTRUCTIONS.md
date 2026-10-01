# Fresh Session Exam v2 — New Chat Instructions

Use no prior-chat history. Work only from the canonical repository and current tracked knowledge.

1. Read `docs/NEW_CHAT_ENTRYPOINT.md`, `docs/FRESH_CHAT_MASTERY_PROTOCOL.md`, and `docs/KNOWLEDGE_RETRIEVAL_CONTRACT.md`.
2. Run `python3 tools/fresh-chat-mastery-gate.py`; require READY.
3. Read `docs/FRESH_SESSION_EXAM_V2.json`. Never inspect `.audit/fresh-session-exam*`, grader internals, deleted blobs/reflogs, or historical answer-key material.
4. When a question has `retrieval`, run exactly `python3 tools/project-knowledge-contract.py <type> '<query>'` and use that canonical object as the answer. For other categories retrieve from the named evidence source and answer the requested semantic fields.
5. Freeze exactly 40 answers at `/tmp/fresh-session-v2-answers.json` as `{ "answers": { "1": {...}, ..., "40": {...} } }`.
6. Do not grade. Report only SHA256 of the frozen answer file.
