# Client automation failure isolation — 2026-10-07

## Goal and baseline
User authorizes fixing the confirmed fleet Output outage and restoring normal automation. Baseline c494fbc; API/worker fc97105, static1384d16, schema160. The per-panel native Sanaei runtime circuit at21:57:16Z incorrectly caused persistent global gate closure. Residential600-second clients expired without refill. Preserve profiles, ownership, expiry, account settings, routing, finite budgets and explicit canary behavior.

## Implementation
Migration161 adds durable per-panel COOLDOWN/QUARANTINED health and safe global gate failure metadata. Automatic lifecycle jobs whose runtime could not be acquired due known API/circuit unavailability are returned to PENDING, with30–120 second panel cooldown. No client mutation occurred; the claim attempt is released and a separate deferral counter is recorded. Authorization budgets are NOT refunded or reset. Claims, planning and admission respect the panel fence across restart.

Known panel request, verification, missing-inbound and rejected-inventory failures use existing fresh-read reconciliation and at most3 mutation attempts. Exhaustion quarantines only that panel; identity conflict quarantines immediately. Job outcome and panel state commit atomically. Quarantine never expires or clears automatically. Success can clear only COOLDOWN. Unknown/internal/journal errors still globally fail-close, preserving safe panel/job/reason metadata. Explicit non-lifecycle canary errors keep their original behavior.

Native HTTP session errors now carry ErrSessionRequest provenance, preserving underlying cancellation/transport identities. This prevents network read/backoff cancellation from being confused with internal database failures; POST requests still have exactly one transport attempt and use existing read-before-retry.

Authenticated Configs and Output display automation status, waiting/quarantined panel counts and safe reasons. Public subscriptions and Output eligibility remain unchanged: URI-only, active, fresh, correctly routed and profile-owned. No server cloud test fixtures, credential changes, blanket retries or safety-gate bypass.

## Verification
Acceptance script runs real isolated PostgreSQL/race client lifecycle tests, admission/resume/status guards, focused API regressions and full go test ./.... Original incident test puts a failed panel first, proves healthy panel completion, durable cooldown after manager restart, then8 competing executor calls create exactly one replacement batch. Fault rollback proves job/health atomicity. Deferrals consume finite authorization budget while retaining a separate count; quarantine cannot be removed by success or generic resume. Lost HTTP POST response does not cause a transport retry.

First OpenAI source review required delayed quarantine for stale inventory/inbound failures; this was implemented and threshold-tested. Final verdict/deployment evidence are recorded in the acceptance JSON, not implied by this document.

## Deployment and recovery
Build API/worker from a clean detached source checkpoint, back up current binaries/static/schema and operator controls, apply additive161 and atomically deploy with readiness/hash checks. Do not downgrade a running, enabled system while ignoring new quarantine state. The initial rollout starts with the already-closed main gate; automatic rollback before resume preserves that gate.

After deployment use existing authenticated POST /api/v1/config-capacity/resume or Configs > Resume automatic clients. This authorization exists in the user's request, but authentication must still be respected. Do not manufacture an admin principal/token or raw-rewrite the gate. Resume does not clear quarantines, historical failed jobs, budgets or expiries. Verify native API-derived creation and class share responses through an actual Residential expiry/replacement cycle before claiming live recovery.

Cloud browser access to the control panel currently fails ERR_BLOCKED_BY_CLIENT; this is a browser network limitation, not proof the service is down. If a legitimate authenticated session is unavailable, finish deployment/evidence and ask the user to press the existing Resume button as the final step.

## Separate unresolved issues
The UpCloud Desired5/capacity2 retirement deadlock and trial-incompatible residential proxy ports, app/recovery.go ambiguous next_retry_at, older x-ui restarts, and Vultr232 credit error classification are separate. This change does not claim to repair those or guarantee output when all providers/panels are unavailable.

Additional review corrections: all remote settings/HTTP response errors retain panel provenance; success clears only its own last_job_id cooldown. Missing/null/non-array client inventories, including non-target inbounds, never prove absence or permit cross-inbound email deletion. Lifecycle bulk reports retain only counts and safe codes; raw returned skipped reasons/errors are omitted. Deferral counters survive POST intent and verification reports. All original review/fault results remain in the evidence directory.
