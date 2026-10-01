# Deterministic Knowledge Retrieval Contract

For fresh-session exam and high-confidence project questions, do not copy arbitrary rich Brain objects. Retrieve the canonical semantic answer shape with:

- DB: `python3 tools/project-knowledge-contract.py db TABLE.COLUMN` → `added_by, readers, writers`
- Route: `python3 tools/project-knowledge-contract.py route 'METHOD|/path'` → `handler, file, line` where file/line are the handler definition, not registration location.
- State: `python3 tools/project-knowledge-contract.py state OWNER_FUNCTION` → `function, file, line, writes, guards`.
- UI: `python3 tools/project-knowledge-contract.py ui FUNCTION` → `function, file, line, dom, loading, errors`.

These shapes intentionally match the semantic fields tested by the blind challenge. Richer Brain objects remain available for investigation, but should not replace these canonical answer fields when a question asks for these chains.
- Intent: `python3 tools/project-knowledge-contract.py intent REQUIREMENT_ID` → `area, requirement, parents`.
- Exact line: `python3 tools/project-knowledge-contract.py line 'FILE:LINE'` → `text, sha256, commit`.
- Test: `python3 tools/project-knowledge-contract.py test TEST_NAME` → `file, line, calls, assertions`.
