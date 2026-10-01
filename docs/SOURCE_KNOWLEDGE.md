# Full Source Knowledge Snapshot

`FULL_SOURCE_KNOWLEDGE.json` is a commit-pinned, line-level snapshot of the project source and operational text files. Each indexed file contains its path, SHA256, byte/line count, exact line text and Go symbol anchors where applicable. `SOURCE_CATALOG.json` is the lightweight file/symbol catalog.

Direct lookup: `python3 tools/source-lookup.py <file> <line> [end-line]`.

This lets a fresh session answer exact questions such as “what is line 105 of internal/adminapi/server.go?” from the snapshot without reopening the working source file. The answer must state/check the snapshot commit because line numbers change across revisions. Exact current working-tree truth still requires comparing the snapshot commit/hash with Git/source.
