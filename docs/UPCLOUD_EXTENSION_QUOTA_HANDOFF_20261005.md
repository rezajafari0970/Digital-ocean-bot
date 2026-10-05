# UpCloud additional null quota fix — 2026-10-05 UTC

Runtime API/worker: 1ae912189edcac45437b1ede21f17743cd3f6942, clean detached build. Static remains cb131196167337612de85fc44d3005ffa9eae7ee, migration 153. Read UPCLOUD_EXTENSION_QUOTA_ACCEPTANCE_20261005.json.

Confirmed evidence: authenticated traces 611a4eddb6ea2e22 at 21:30:37 UTC and c7a50ce4ec0ce473 at 21:31:04 each reported exactly one quota issue: INVALID_QUOTA_OTHER_FIELD_NULL, at account, HTTP200. This establishes an unrecognized additional null quota as the current account parse blocker; it is not merely the earlier nullable-Dev hypothesis. Raw response/key/token were not collected and the actual extension name is not known.

Changed:
- Additional null quota entries are retained as present nil. They are never deleted, changed to zero or treated as unlimited.
- Known strict quota fields still reject null. The known schema (26 strict fields plus two nullable Dev fields) is centralized and supplies the safe diagnostic name allowlist.
- Usage decoding uses the same presence-aware map, so unrelated additional null usage entries do not poison ordinary-plan budgets.
- A selected dynamic/future plan with a present nil quota or usage still fails closed (UNKNOWN_PLAN_QUOTA / UNKNOWN_RESOURCE_USAGE); missing required budget values reject; zero remains known zero.
- Negative, fractional, nonnumeric and overflowing quota values still reject. Raw unknown names/values remain private.
- No DO/Vultr, routes, proxy identity, gates, schema, static or permanent residential profile changes.

A synthetic future-key fixture reproduced the exact old diagnostic. After the patch, Account -> Capacity -> Catalog preview and ordinary-plan capacity pass. Selected future-plan null quota/usage cannot authorize capacity. Full Go, race with isolated PostgreSQL, mobile/browser and before/after configuration checks pass. OpenAI planning/orchestrator COMPLETE. The plan initially miscounted the unchanged schema; the job context now records the actual 26 strict plus two nullable fields.

Production API/worker are healthy and configuration is preserved. Initial acceptance was blocked by the existing DataImpulse proxy health gate; a fresh status read showed healthy again, and one read-only acceptance retry reached UpCloud and returned the expected synthetic-token HTTP401. Both original blocked evidence and retry are preserved; proxy configuration/gates were not bypassed. Invalid synthetic-token probe only proves reachability and error-reporting; it is not a rejection of the user's credential. No saved UpCloud account existed at baseline and no live UpCloud create/delete was performed.

Next: user refreshes panel and retries Validate. The observed account rejection is fixed in regression tests, but authenticated complete preview still needs confirmation. If a later stage fails, use the new stage/reason/ref rather than assuming the same account issue. Never request tokens or dump raw account responses.

Evidence/backups: /root/backups/dob-upcloud-extension-quota-20261005. Rollback restores api.before/worker.before (4ada688), then restarts services; no schema/static/config rollback required. Preserve the current permanent residential publication and proxy choices.
