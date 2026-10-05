# UpCloud actual quota failure diagnostics — 2026-10-05 UTC

Runtime API/worker: 4ada688681d03283dc0712c2fb5cc6798f91b3c0, clean detached build. Static remains cb131196167337612de85fc44d3005ffa9eae7ee, migration 153. Read UPCLOUD_QUOTA_DIAGNOSTICS_ACCEPTANCE_20261005.json.

The prior nullable-Dev schema correction did NOT resolve the user's authenticated account preview. User retries d6cbab5416e7982f at 21:14:03 UTC (2877ms) and 37682fd15a53aa98 at 21:14:17 (8035ms) both ran on verified clean runtime 5aa7fef and returned account HTTP200 INVALID_NUMBER. Do not present the prior null-Dev hypothesis as the actual cause. The historical raw response was not retained; no saved UpCloud account existed at baseline.

This release is diagnostic only:
- Invalid quota failures now identify an allowlisted field name and fixed value shape/type category in reason.
- All invalid quota issues are sorted and bounded to 64 and carried across HTTP-status normalization into admin JSON and a single correlated safe journal line.
- Unknown key names become OTHER_FIELD; raw keys, values, account identifiers, credits, tokens, proxy secrets, response bodies and headers are never emitted.
- Export revalidates vocabulary and length. Unknown or injected metadata is dropped.
- Existing integer parsing, two nullable Dev fields and capacity fail-closed semantics remain unchanged. No speculative null-to-zero, negative-to-unlimited or rounding change.

The fixture-based client/decoder/admin tests verify multiple issues, HTTP200 preservation, deterministic ordering, bounded output, metadata injection rejection and secret redaction. Full Go, race with isolated real PostgreSQL, browser and config comparison passed. OpenAI-backed orchestrator COMPLETE.

Next necessary input: user retries Validate once in the panel and shares the new reason/ref with token hidden. Read the matching provider_preview_failed journal including quota_issues. That evidence should identify all invalid fields in one attempt. If the field is OTHER_FIELD, do not dump arbitrary raw response keys; add a narrowly authorized safe field fingerprint/allowlist only if needed. Do not guess credentials from screenshots or request tokens in chat.

There is no actual authenticated preview success or proven root fix yet. Synthetic invalid-token live probe is reachability/staged-error proof only, never evidence that the user's token is invalid. No UpCloud cloud create/delete was performed.

Evidence and rollback binaries: /root/backups/dob-upcloud-quota-diagnostics-20261005. Restore api.before/worker.before (5aa7fef) and restart services if required; no schema/static/config rollback. Active permanent residential publication and proxy/gates preserved.
