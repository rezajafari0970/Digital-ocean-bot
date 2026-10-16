# Server protection reliability — 2026-10-16

Scope: reduce protection/control-plane overhead and listener bootstrap starvation; preserve real resource-pressure admission and accurate health reporting. User requested implementation without external model API calls. No model API was used; no independent model review is claimed.

## Evidence and diagnosis

- Base source fba86a0fdb344c97735b29fb39aed44b51ea99fd; installed public rollback 936896afc1d1301bfcfc10297db1d16446af8bcb on both hosts.
- Read-only native inspection proved UNSUPPORTED nodes really had only the localhost management tunnel, no public VLESS inbound. XHTTP was already supported by port detection; transport-parser replacement would not fix this.
- Initial listener setup shared the two-slot maintenance class with complete established-fleet reconciliation and capacity scans. Some unconfigured nodes were tens of minutes old. New bootstrap work receives one separate lane while retaining the same ten-slot shared SQL budget and the same structural lock.
- Periodic global reconciliation previously occupied admitted work while waiting for long config-lock owners. It now tries the existing lock without queuing; a later sweep retries. Mutations still execute exclusively under that lock.
- Every guardian poll previously made inventory and installer SSH calls, configured files and called systemctl start even on a healthy unchanged installation. A single bounded read-only probe now verifies binary, policy and owned-unit hashes, persistent enablement and a fresh running-agent receipt. Any failed installation proof falls back to the guarded installer. Actual CRITICAL/UNSUPPORTED/ERROR observations remain visible; resource pressure does not trigger needless reinstallation.
- wait-status previously accepted a fresh cached sample even when the actual agent lock proved it was stopped. It now requires the requested enabled policy and a running agent. Validation errors identify missing agent/firewall/listener instead of confusingly quoting a healthy resource reason.
- Disabled/deleting accounts are excluded from ready-panel polling and bootstrap.
- UI distinguishes resource pauses, overdue protection reports, missing listeners and sticky output withholding; no health count or freshness limit was loosened.
- Twelve pre-release fleet samples showed 54–55 eligible protected nodes, 36–41 fresh receipts, 7–8 errors (missing listeners), 6–7 admission pauses and zero overdue provider accounts. Native pressure samples confirmed real CPU, memory and connection-table pressure on other nodes.

## Invariants and rejected approaches

No protection disable, no kernel limit increase, no Xray replacement for this repair, no provider mutation, no deletion of users or active sessions, no parallel native routeTest requests, no widening of 15-second output or 60-second routing/protection freshness. Unverified blocked admission remains sticky. Existing policies, listener pins and management exclusions are preserved. New listeners continue to follow the user's existing creation policy.

A zero-warning promise is not meaningful under real resource saturation, external provider outages, or normal initial provisioning. Acceptance must distinguish software faults from correctly enforced resource pauses. No attempt is made to hide those pauses or pretend stale observations are current. The separately recorded long residential config-lock/output-coverage issue is not declared fixed by this change.

## Verification

Evidence is retained under `.local/evidence/` in the task worktree, with summarized acceptance committed separately. A new steady-poll regression failed on the baseline. Corrected focused, real PostgreSQL, race, browser and isolated network/mount namespace tests exercise one-call polling, invalid proof fallback, stopped-agent rejection, bootstrap/maintenance separation, disabled/deleting account withdrawal, config-lock exclusion, firewall behavior, established sessions, singleton ownership, policy fencing, cleanup and bounded recovery. Two preliminary PostgreSQL fixture failures were missing required fixture columns, corrected without changing runtime schema.

Full project tests, vet, normal publisher/exact Ubuntu install, main/stable CI, secondary-host update, one-node guardian canary and final fleet sampling are required before deployment acceptance. Existing canonical bootstrap remains blocked by its pre-existing detached HEAD; no full-project mastery/READY claim is made.
