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

## First release and native bootstrap follow-up

First product commit 8ceb426c9331af624fccef344f9e18d4f46212ea was published as 61e398be9f6d87c5b3f9b7335018276098598da0. Full Go tests/vet, normal publisher/exact Ubuntu installation and both main/stable CI passed. Both controlplanes were updated and verified with five readiness samples, matching running binary hashes, unchanged schema/accounts/environment/protection/creation/output policies and retained listener pins. The single guardian canary passed running/on-disk binary identity, fresh receipts and Xray process age predating deployment; its temporary upgrade filter was removed before fleet release.

Steady fleet verification found a second bootstrap dependency: native panel target scans succeeded quickly while SSH discovery/key generation repeatedly timed out. Bootstrap now bounds native discovery to eight seconds, uses the authenticated native session for inventory and target scans, and retains pinned SSH fallback plus the existing repair fence/cooldown when native discovery is unavailable. It uses the same local secure X25519 generator already used by global REALITY reconciliation; Registry.Ensure retains existing credentials and locks new creation. Three stability observations and the selected-target full scan remain mandatory.

The new actual-PostgreSQL regression failed on baseline with SSH unavailable although native HTTP was healthy. It covers convergence without any SSH reads, malformed scan rejection without credential creation/repair, idempotent credential retention, and unavailable-native fallback with repair cooldown. Two preliminary fixture issues (UUID parameter typing and absent policy seed) were corrected, not classified as product failures. First-rollout receipts temporarily lagged during upgrades; a later snapshot reached 54/57 fresh receipts, with only three new missing-listener nodes. Final fleet acceptance is still pending; true resource-pressure pauses must remain visible.


## Provider cadence transition correction

Two samples in the 36-sample steady fleet window briefly marked a DigitalOcean account overdue, each resolving in the next five-second sample. The previous observations were approximately 205 and 299 seconds old: valid for the idle 300-second interval but relabeled against 60 seconds before the scheduled refresh completed. The coordinator wrote the new diagnostic interval before acquiring the refresh lock or observing the provider. A PostgreSQL regression with another worker holding that lock reproduced the premature 300-to-60 write.

Cadence is now attached to the scheduled attempt and committed in the same transaction as its accepted observation or error. Either direction of cadence change requires one real observation before the normal freshness skip applies. The prior observation retains its actual prior cadence during the read; no timestamp or expiry margin is extended. The transaction enforces account/credential validity and ordering; older independent restriction/error evidence cannot overwrite a newer schedule. Failed snapshot insertion rolls cadence back too. Unscheduled callers leave cadence unchanged. Provider backoff, real error display, 1–300-second bounds and strict pre-create freshness remain unchanged.

Final verification includes successful and failed observations, rejected credentials, obsolete results, older independent restrictions, invalid intervals, no premature write under the provider advisory lock, and rollback without fabricated freshness.

## Rolling scheduling and executing-image follow-up

Native release8225c31/public8abcacc was published and installed on both controlplanes; all three service executable hashes and five readiness observations per host passed. Retained pins and policies stayed unchanged. Fleet monitoring did not establish steady complete freshness, so acceptance stayed open.

The later video showed58/64 fresh, six overdue, ten genuine pressure pauses. A new baseline test reproduced healthy peers waiting for one slow enabled node. Production now has a candidate rolling dispatcher with six enabled/two cleanup slots, separately admitted discovery under shared budget10, bounded SQL/attempt deadlines, oldest-due selection/exclusions, local contention cooldown, revision/due fences and joined cancellation. A stress test caught422 contention retries/300ms in the first candidate; bounded cooldown fixes it. Real PostgreSQL/race tests cover129 nodes and two healthy renewal cycles while a peer remains blocked.

The actual staging recipe reproduced unrelated-release artifact hash churn on both architectures. Candidate guardian builds omit release linker/VCS stamps and sanitize build environment; the manifest retains release identity. Production-stage tests now show stable bytes across unrelated commits and changed bytes for reachable Version changes. This requires one staged artifact transition.

Candidate runtime proof binds persisted samples to writer PID/start ticks/executing inode digest. ReadStatus derives public proof from the kernel; inventory brackets it with systemd PID/invocation identity. Initial fast observation and post-install acceptance require the approved artifact. A live unproven/old producer with target bytes on disk is reported without blind reinstall. Real inode-replacement and process-death tests pass. Legacy staged-out agents retain policy receipt semantics until explicitly upgraded; canary acceptance separately attests actual image.

Four independent Astra max plans completed after the initial12000-output cap exhausted reasoning without final answers. Raw initial and continuation responses are retained. Known eight-call usage214140tokens, estimated standard costUSD5.6680145 including cache-write pricing; a ninth final-code review and four provider-isolation plan calls are pending. No model approval substitutes for tests.

Focused race,129-node scheduling, kernel namespace/nft faults, stable artifact controls and govet passed. One initial inode fixture failed because GNU multicall coreutils requires argv0 sleep; corrected to preserve argv0 and use a different replacement executable. Full regression and final code review are still pending. Rollback remains public8abcacc/source8225c31.

Live provider warnings recurred with five overdue accounts, twenty stale-provider server statuses, and an actual proxy failure. Source shows serial account refresh under one batch-wide control admission. This is a separate remaining cause, now isolated on fix/provider-observation-20261016; freshness thresholds, proxy circuit gates and provider-error semantics must remain unchanged.

### Third candidate verified 2026-10-16T05:22:23.445136+00:00

Exact-source full regression and vet passed after B1 fix. Astra final review PASS closes B1: live or unknown systemd process with unproven/malformed sample cannot trigger installer. Stable PID0 permits stopped-agent repair. Rolling scheduler, real kernel producer proof and deterministic guardian artifact gates passed. API cumulative usage across guardian and provider planning: {"requests": 14, "input_tokens": 226183, "output_tokens": 194512, "total_tokens": 420695, "estimated_usd": 11.569912}. Deployment and fleet verification pending; provider scheduling and residential lock issues are not closed.
