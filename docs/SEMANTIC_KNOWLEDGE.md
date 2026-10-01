# Semantic Source Knowledge

Commit-pinned semantic/navigation layer for **409 Go files** and **1537 symbols** at `1d412f50d6b347a99f8930f095d9fd3d7f9f058c`.

Records file roles, imports, symbol ranges, lexical calls, candidate caller files, SQL tables and endpoint literals. Candidate callers are navigation hints, not a compiler-proven call graph.

## Major package roles
- **API process entrypoint** — 1 files
- **DigitalOcean provider implementation** — 16 files
- **HTTP/Admin API handlers** — 50 files
- **Sanaei/x-ui integration** — 51 files
- **Vultr interactive browser/console integration** — 3 files
- **Vultr provider implementation** — 11 files
- **account identity/isolation** — 6 files
- **application orchestration/composition** — 31 files
- **compute lifecycle/reconciliation** — 11 files
- **network/proxy isolation and health** — 34 files
- **panel lifecycle/policy/inventory subsystem** — 45 files
- **project implementation/support** — 143 files
- **provider contracts/registry/common behavior** — 6 files
- **worker process entrypoint** — 1 files

## Retrieval workflow
1. Feature ownership: `FEATURE_FLOW_INDEX.md`.
2. Semantic relationships: `SEMANTIC_KNOWLEDGE.json`.
3. Exact definitions: `SYMBOL_INDEX.json`.
4. Exact source lines: `FULL_SOURCE_KNOWLEDGE.json` / `source-lookup.py`.
5. Verify snapshot commit/hash against current Git before treating it as current truth.
