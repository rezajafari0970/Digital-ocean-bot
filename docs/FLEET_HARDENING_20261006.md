# Fleet hardening — 2026-10-06

This is a source delta over runtime 5628300 / canonical bc3651e, not a replacement for frozen Project Brain history. See the paired SOURCE_SNAPSHOT and ACCEPTANCE JSON. Deployment evidence is recorded separately; a passing fixture is not live provider proof.

## Changes and boundaries

| Audit area | Implemented mitigation | Remaining boundary |
|---|---|---|
| Initial SSH rejection | Validate/read back provider SSH key material; cloud-init assigns managed key explicitly to root and default user | Actual provider guest boot/network can still fail |
| Indefinite boot retries | 15-minute initial SSH budget after at least 3 finished failed probes; durable lease/transaction; owned retirement through ordinary lifecycle | Never expires interrupted scripts or unknown provider mutations; deadline checked on recovery discovery |
| Slow recovery blocks other accounts | Persistent dispatcher, total 6 / per-account 2; interleaved discovery and independent lifecycle loop | Provider and network delays remain |
| Blocked servers republished after stale poll | Sticky last verified receipt; failure, age and disable intent cannot erase actual admission block | Fresh verification/cleanup required to release; this is deliberately conservative |
| Stale/incorrect dashboard health | Latest finished panel inventory + fresh provider proof + applied routing; explicit degraded/admission-blocked and restricted account counts | Control-plane proof is not mobile end-to-end traffic proof |
| Failed deployment history hidden | Installer failure/rollback states included; read/scan errors surface instead of misleading zeros | Historical incidents retained, not rewritten |
| Account Active but unavailable | Warning from canonical provider observation displayed independently from readable account state and scheduler capacity | UpCloud trial/permission and provider quota restrictions cannot be bypassed |
| Repeated boot failure | Account/region/plan/image circuit after 2 boot-budget failures, 1h cooldown, single half-open build; completed SSH clears boot circuit | Installation and other failure classes retain their existing budgets |
| Slow serialized client queue | Drain ready work without a mandatory 1s delay after every item; bounded batch/time; every claim rechecks durable gate | Mutation concurrency remains 1; batching does not guarantee 50ms |
| x-ui active but Xray missing | Managed-process and owned-listener proof; partial loss is degraded; missing process recovery requires continuous 60s low-pressure window, valid config, locks and restart budget | Existing client sessions cannot migrate transparently; restarts can interrupt sessions |
| Per-process descriptor exhaustion | Track max managed x-ui/Xray fd usage versus process RLIMIT; feed 90% pressure threshold | Sampling is not predictive certainty |
| Unbounded observations/pool | Bounded pruning of old superseded successful observations; preserve failures/mutation journals/latest evidence; API pool24 / worker64 and proxy maintenance concurrency4 | PostgreSQL default100 assumes one API+worker; additional processes require budget review; failure evidence still grows |
| False service readiness | Database plus worker heartbeat and recovery/scheduler/lifecycle progress | Not a multi-controller HA design or full fleet data-plane SLO |
| Unsafe broad binary rollout | Guardian binary allowlist via DOB_GUARDIAN_UPGRADE_PANELS; existing policy still reconciles while upgrade is paused | Rollout control does not replace configuration scope |

## Preserved contracts

Desired is a hard ceiling, not a guaranteed number of simultaneously READY servers. Existing Apply-to-existing behavior, lifecycle timing, account deletion fencing, provider-confirmed cleanup/backfill, immutable deployment snapshots, unknown mutation reconciliation, SSH host-key pins, residential routing/publication, enabled profiles, client gate and credential identities are preserved. Scheduling now atomically reserves the selected snapshot and rules revision, holds a run lease through SSH-key preparation, and enforces max concurrent builds under admission lock.

No FinalMask, P3/P4/P6 or other removed transport experiments are restored. No synthetic paid cloud resources are used for acceptance. Natural lifecycle work may retire already failed owned builds under the new budget.

## Dependencies this source change cannot complete

- Mobile/client automatic failover and session reconnection need the app source and client behavior. Existing TCP sessions cannot be moved to another host by changing subscription output.
- A second controller, independent database/failover and load-balancer infrastructure are needed to remove the management single point of failure.
- Public management TLS/domain/access migration needs a chosen hostname and access plan; do not silently close the operator's only access.
- Keeping the full READY target during replacement requires explicit paid surge/headroom. Do not exceed Desired without authorization.
- Central proxy reachability/RTT does not measure ad load or residential throughput. Preserve server-local routing health; do not promise AdMob success or universal 50ms recovery.

## Verification and rollout

Full Go suite, focused race + isolated PostgreSQL fault tests, and opt-in guardian tests in isolated network/mount namespaces are required. Production indexes are created concurrently before migration159. Schema is additive and rollback retains durable evidence.

Build only from a clean release worktree after orchestrator CHECKPOINT. Back up production binaries/static/manifest and database; install API/worker atomically; verify running executable hashes. Roll guardian binaries 1 → 6 → all after receipt/process/listener checks. Do not reset user controls to old snapshots; an independently changed operator setting is not an excuse to overwrite it.

Evidence and rollback: /root/backups/dob-hardening-20261006. Binary rollback restores the previous API/worker/static/guardian artifacts and removes only the temporary rollout override. Keep migration159 and evidence; never restore a stale database over ongoing lifecycle/provider operations.

## Completed deployment evidence

Runtime 8e75d91c0f12573229abbbf51362995a45203f5a deployed with migration159. See ACCEPTANCE JSON for exact timestamps, account population, provider restrictions, rollback attempts and proof boundaries. All binaries were built from the clean release worktree; the subsequent upgrade-script safety fix only affects future operator deployments. Existing operator settings were compared exactly and preserved.
