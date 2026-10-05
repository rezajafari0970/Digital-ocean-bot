# UpCloud preview diagnostics continuation — 2026-10-05 UTC

Runtime API/worker: 88f8b50f5d0d3107069c2e495d6b5cc71cb2416b, clean detached build. Static UI unchanged from cb131196167337612de85fc44d3005ffa9eae7ee; migration remains 153. Read UPCLOUD_PREVIEW_ACCEPTANCE_20261005.json.

The user submitted UpCloud preview failure through DataImpulse: generic Could not reach provider. Historical preview logging contained no stage. The user's token existed only in the unsaved browser form and was not retained server-side. Original authenticated failure is **not yet diagnosed**.

Read-only tests through the exact selected SOCKS5 proxy and existing preview sticky identity reached api.ipify.org (200) and UpCloud (401 without token) in about 1 second initially, then 120 ms over the same connection. During first rollout verification the proxy health gate rejected with proxy_not_healthy; later it returned healthy. First rollout automatically restored prior binaries. This is evidence of recorded health instability, not proof of the screenshot's cause.

Changed:
- Separate malformed/empty/oversized/schema-invalid responses from connection errors while keeping them retryable; create ambiguity remains protected.
- Safe UpCloud stage/account-capacity-catalog, operation, provider HTTP status, allowlisted reason, generated request ID and elapsed time. No raw error/body/header, token or proxy credentials in diagnostic UI/journal.
- Empty catalog reports counts only. Unknown API error codes become HTTP status codes rather than arbitrary provider text.
- Existing other-provider error behavior, selected gateway, sticky identity, fail-closed gates, quota strictness, retries and resource cleanup unchanged.

Full Go/race/PostgreSQL/provider faults/redaction/staged handler/mobile tests pass. Final deployment passed configuration comparison and authenticated local preview with a deliberately invalid synthetic UpCloud token over the selected proxy. It returned provider HTTP 401, stage account, AUTHENTICATION_FAILED and trace bf3359dc98d1438d, correlated in journal. This is reachable-provider/error-reporting proof, **not validation of the user's token or a real account/cloud lifecycle**.

Next: user refreshes panel and retries Validate with their existing token. Read the safe provider_preview_failed journal entry using the displayed ref. Determine the actual failing stage before changing response parsing or proxy configuration. Do not request token in chat or infer token invalidity from the synthetic probe. Duplicate proxy selections are not distinct backups; no selection was changed.

Evidence/backups: /root/backups/dob-upcloud-preview-20261005. Rollback restores api.before/worker.before (prior cb13119), restarts services; no schema/static/config rollback required. Prior deploy attempt log retained. Active permanent fleet residential profile and all compared settings/gates/endpoints preserved.
