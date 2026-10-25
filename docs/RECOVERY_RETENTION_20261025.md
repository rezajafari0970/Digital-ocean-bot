# Recovery, retention and runtime memory — 2026-10-25

This continuation repairs incomplete database reads, observation cleanup capacity and retained panel-runtime identities. The whole-project review remains incomplete. Operational acceptance is recorded in the companion JSON; the original database remains quarantined.

## Result and code paths

| Finding | Final behavior | Verification |
| --- | --- | --- |
| Interrupted UPDATE RETURNING could confirm resource deletion with incomplete deployment receipts | confirmDeletedTx checks rows.Err before committing any deletion state or event | Real PostgreSQL faults before and after the first returned row, full rollback, successful retry and duplicate confirmation |
| API and background readers could accept partial SQL results | Query, Scan and iteration errors stop the affected response or batch; deployment image history uses the existing transaction | 55 API query/stream/scan/close fault cases; full source/native/race gate |
| One thousand observations per five-minute tick could fall behind live generation | At most ten 1000-row batches per table, round-robin, within the original shared ten-second deadline | 15000 expired-row fixture, cancellation, upper limit, second-round catch-up and retained evidence |
| Missing statistics could produce repeated bitmap scans and sorts for each latest-witness lookup | Ordered witness probes and migration 197's completed-only index provide an appropriate access path before ANALYZE | Independent exact-SQL plans and unmodified backlog deadline |
| Runtime entries and mutation locks could accumulate across panel churn | Acquisition sweeps expired cache/circuit identities; reference-counted mutation gates exist only while callers or waiters use them | Failed opens, 1000 identities, retained/new runtime serialization, independent panels, cancelled waiters and panic cleanup under race |

Affected SQL boundaries include application deletion confirmation, catalog/provider collection, deployment image selection, consistency reconciliation, proxy monitors and the reviewed admin list/output paths. The lexical inventory found 20 additional missing iteration-error candidates after the deletion path; after correction that pattern returned zero candidates. This is a targeted pattern scan, not proof that every SQL path or every repository file is correct. Independent consistency upserts remain independent and idempotent; they are not claimed to form one transaction.

Retention preserves the latest successful panel evidence, the latest canonical provider observation, all failure/running observations and snapshots referenced by capacity events. The new index preserves existing rows and the broader recent-inventory index. PostgreSQL autovacuum can reuse deleted-row space; deletion is not an assertion that filesystem usage immediately shrinks.

## Reproduction and acceptance

- Negative source run [40218138394](https://github.com/rezajafari0970/Digital-ocean-bot/actions/runs/40218138394) reproduced incomplete deletion confirmation and insufficient catch-up.
- Intermediate runs 40219030543, 40219519879 and 40219845394 failed the unchanged ten-second retention limit. The failures were retained, not converted into success through retries or a longer acceptance timeout.
- SQL diagnostic [40220741980](https://github.com/rezajafari0970/Digital-ocean-bot/actions/runs/40220741980) showed the stale-statistics bitmap/sort plan. The panel batch exceeded its separate fifteen-second diagnostic statement limit; after ANALYZE it completed quickly.
- Indexed diagnostic [40220963291](https://github.com/rezajafari0970/Digital-ocean-bot/actions/runs/40220963291) completed the same 1000-row panel batch before ANALYZE in 34.605ms and the provider batch in 13.785ms. These are disposable-fixture observations, not a production fleet speedup claim.
- Final source [40220965327](https://github.com/rezajafari0970/Digital-ocean-bot/actions/runs/40220965327) passed full Go/vet/browser checks, 230 mandatory roots with zero skips, native transports, race detection, 20 lifecycle isolation repetitions and the disposable restore/interruption drill at schema 197. The complete backlog test, including fixture work, passed in 1.50s.
- Public [40221257928](https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40221257928) passed source checks, fresh Ubuntu 24.04 installation and repeated installation with identity preservation.
- Atomic [40221609420](https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40221609420) promoted main and stable with expected-head leases; both refs were independently checked.

Product a400e455a67bc95fa51cbafb77b22b40508c66c5 merged through [PR13](https://github.com/rezajafari0970/Digital-ocean-bot/pull/13) as 11b390020a9e611f564167c0e04fd76552b45475. Public revision is 53294d3936a96fa342db4036fa87f910f289e1e9. Its immutable export contains 1745 files and passed both live-secret-set comparisons and Gitleaks. No operational data, credentials or private receipts were exported.

## Live rollout

Both bounded updates completed successfully. Default and production run public 53294d3936a96fa342db4036fa87f910f289e1e9 at schema 197. Each instance passed all 15 preservation checks, all three running-binary hashes and all five readiness checks. Served UI assets match the candidate. The production baseline contains 6183 profile identities and 77 Trojan identities; no baseline identity disappeared and no unexpected credential/ownership change occurred. Five profile retirements had independent confirmed-expiry evidence; six new profile identities appeared during normal operation. All 77 Trojan identities were unchanged.

The controlled production API stop lasted 216.581 seconds (systemd stopped-to-started timestamps). This was a maintenance update, not zero-downtime deployment. HTTPS frontend version, readiness, certificate validation and configuration UI asset identity passed.

At 2026-10-25T18:49:42.316682+00:00, the new production worker had recorded 1 successful normal retention event(s), deleting 1867 observations; 8 panel observations remained eligible at the sampling instant. Events are filtered from the new worker start, excluding the previous worker. These observation rows are not user configurations. The latest integrity/backup receipt is healthy, recovery_required=false, and the current database checksum-failure counter is zero. The sample had 7859412992 free disk bytes and 1647460 KiB available memory; these are point-in-time measurements, not capacity guarantees.

The new production baseline backup is 615,107,439 bytes, with 6,183 profile identities and 77 Trojan identities. Missing baseline identities now block acceptance. Expected expiry retirement is accepted only with separate provider-confirmed deletion evidence and unchanged credential identity. Additions are reported separately.

An older 614,941,190-byte routine backup was copied to the original host through the existing restricted read-only backup identity. Destination SHA256, fsync, exact local size/mtime/link identity and a second source digest were checked before unlinking the redundant local copy. The current backup and all incident archives stayed local. A durable archived-pointer receipt remains at the old path. This complements the seven earlier relocations; it does not resolve long-term storage capacity by itself.

Private receipts: /root/dob-recovery-followup-20261025 on the recovery host. Archive receipts: /root/dob-config-cleanup-archive-20261025 on the original host. Do not delete these as disposable test output. The source of truth for exact rollout results and observation times is the companion JSON.

## Remaining boundaries

Three requested configuration deletions were still waiting on one expired AWS server during diagnosis. The panel is enabled and deployment generation matches, but panel TCP and SSH timed out from both controllers. AWS returned InvalidClientTokenId/GetCallerIdentity. A timeout or invalid credential does not prove absence; do not force retirement. Restore authorized account/network access through the existing account workflow, then allow the durable cleanup request to resume. The final observation is in the companion JSON.

Historical original-host PostgreSQL logs contain invalid-page/checksum failures beginning 2026-10-22. This confirms the earlier corruption incident, not its hardware or storage root cause. Original cluster18/main, application workers and publisher remain inactive. No original database was restarted, repaired in place or overwritten. Current readiness and healthy recovery storage are separate from resolving that quarantined incident.

No new paid OpenAI generation was sent. The original schema 2 ledger still has one unresolved request and 1,501,011 microUSD reserved under its existing budget. The earlier read-only usage request was forbidden; no usage or charge was invented. Do not reset the ledger or replay the uncertain operation.

Full manual coverage is 269 current files / 31,720 lines and 214 of 1,218 baseline Go file paths / 25,264 baseline lines. Partial reads remain explicitly listed in SYSTEM_AUDIT_RESTART_20261025.json. No universal error-free, indefinite uptime, Android-device acceptance or measured fleet deletion-latency claim is made.

## Continuation

Read the companion acceptance JSON and current PROJECT_CONTEXT/PROJECT_STATE/HANDOFF first. Keep the recovery host's bounded production workload; run heavy native/race/database tests on independent runners. Continue the pending module inventory and resource/provider ownership audit. Any future change must retain the regression gate, exact-source install test, immutable export scans, backup and runtime identity checks.
