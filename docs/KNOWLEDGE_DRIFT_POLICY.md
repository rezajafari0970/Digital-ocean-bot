# Knowledge Freshness / Drift Policy

A knowledge artifact is current when its `source_commit` is an ancestor of HEAD **and no relevant product/source path changed** between that commit and HEAD. Documentation/knowledge-only commits may advance HEAD without invalidating a source snapshot.

Relevant indexed paths are `cmd`, `internal`, `migrations`, `web/static`, and `deploy`. If any changes occur there, the corresponding brains must be regenerated before a fresh session may treat them as current.

Run `python3 tools/knowledge-drift-gate.py`. Only `ready: true` permits the handoff to claim current source knowledge.
