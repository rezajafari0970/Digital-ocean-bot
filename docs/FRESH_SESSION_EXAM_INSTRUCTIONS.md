# Fresh Session Exam — Instructions for the New Chat

You are being tested on whether a genuinely fresh ChatGPT session can recover this project's knowledge from the repository rather than from prior chat history.

1. Work only from `/root/projects/Digital-ocean-bot-canonical-e2e` on branch `checkpoint/final-e2e-20260929`.
2. Read `docs/NEW_CHAT_ENTRYPOINT.md` and `docs/FRESH_CHAT_MASTERY_PROTOCOL.md`.
3. Run `python3 tools/fresh-chat-mastery-gate.py` and require READY.
4. Read `docs/FRESH_SESSION_EXAM_PACK.json`. Do **not** inspect `.audit/fresh-session-exam/`, deleted Git blobs, reflogs, prior commits containing an exam key, or grader internals/answer material. Do not use this current/old chat history to answer.
5. For every exam question, retrieve evidence through the normal knowledge hierarchy (`project-brain-query.py`, Brain JSONs, source snapshot, exact source where necessary).
6. Create `/tmp/fresh-session-answers.json` with exactly this shape: `{ "answers": { "1": <answer object>, ... } }`. Preserve the answer object's keys/types exactly as supported by evidence. Do not run the grader yourself.
7. Report only that the answer file is frozen and give its SHA256. The old session/operator will grade it separately against the isolated key.

Passing requires all 40 structurally exact evidence answers. This tests retrieval/composition, not unaided token memorization.
