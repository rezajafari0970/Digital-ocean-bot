# Operational repair release — 2026-10-09 Tehran

Read OPERATIONAL_REPAIR_STATUS_FA_20261009.md, OPERATIONAL_REPAIR_ACCEPTANCE_20261009.json and OPERATIONAL_REPAIR_DELTA_20261008.md first. Runtime product 897f408 is verified; this newer checkpoint is documentation only. The 90-percent goal remains unverified. Inspect live status and current canonical/runtime before further work.

---

# Project Brain

This is the unified retrieval layer for a fresh ChatGPT session. It combines the line-level source snapshot, semantic symbol relationships, route index, schema/migration index and test map without requiring broad source reopening.

Use: `python3 tools/project-brain-query.py '<query>'`.

Supported direct forms include `internal/adminapi/server.go:105`, symbol/handler names such as `createAccount`, endpoint fragments such as `/api/v1/output`, table names such as `accounts`, file/path fragments, and exact text fragments. Results are commit-pinned and include the source commit.

Retrieval order for a fresh session: Project Brain query -> semantic/symbol/schema/test indexes -> exact line snapshot -> live source/runtime only when freshness or deeper behavior verification is required. Never answer a line-number question without binding it to the snapshot/source commit.
