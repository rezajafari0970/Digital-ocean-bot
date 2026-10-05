# Development Ready Handoff

## Authority
- Repository: `/root/projects/Digital-ocean-bot-canonical-e2e`
- Branch: `checkpoint/final-e2e-20260929`
- Use the repository and current production-status artifacts as authority; do not rely on unaided verbatim memory.
- `docs/FINAL_KNOWLEDGE_BASELINE.json` fingerprints the deterministic knowledge corpus used for fresh-session retrieval.

## Fresh-session bootstrap
1. Read `docs/FINAL_KNOWLEDGE_BASELINE.json`.
2. Read `docs/CONTINUITY_FINAL_STATUS.json`, `docs/KNOWLEDGE_DRIFT_STATUS.json`, and `docs/PRODUCTION_REVISION_STATUS.json`.
3. For source questions, retrieve from `docs/SOURCE_INTERNALIZATION_BRAIN.json` and `docs/SYMBOL_TOPOLOGY_MEMORY_PACK.json`; inspect live source before mutation.
4. Check `git status`, HEAD, and relevant tests before editing.
5. Continue from the current development frontier rather than replaying memory rehearsal.

## Memory-training disposition
The autonomous spaced-rehearsal service is intentionally disabled. API-runner training is not treated as transfer of memory into the interactive ChatGPT conversation. Historical holdouts remain evidence only and are not rewritten.

## Development gate
Development may resume when the knowledge artifacts exist, their hashes are frozen in the baseline, repository drift is checked at session start, and changes are grounded in live source/tests rather than unsupported recall.

## Latest production continuation (2026-10-03)
- Read `docs/DURABLE_BULK_V3_HANDOFF.md` and `docs/BULK_V3_ACCEPTANCE.json` for the current frontier.
- Durable v3 bulk ten-client canary and cleanup passed; runtime source af1a541799088c6d3c89af675d5f6b02f82cd84c.
- Any following evidence-only commit does not imply binary drift; compare product paths before rebuilding.
- Gates remain closed. Next step is measured chunk-25 acceptance, with bulk policy/expiry lifecycle integration required before fleet enablement.

## Latest ramp checkpoint (2026-10-03)
- Read `docs/BULK_V3_25_ACCEPTANCE.json`: stage 25 is accepted, including complete resource sampling and cleanup.
- Next measured gate is 50 clients. Main/bulk execution gates remain closed; fleet generation is disabled.
- Build on the main disk with a disk-backed GOTMPDIR; /tmp tmpfs exhaustion was observed and resolved for this workflow.

## Latest ramp checkpoint — stage 50 (2026-10-03)
- Read `docs/BULK_V3_50_ACCEPTANCE.json`: one 50-client batch passed create, Output, complete resource sampling and cleanup.
- Next measured gate is 100 clients; current utility is bounded to 10/25/50. Both execution gates and fleet generation remain closed.
- Capacity table timestamp predates this stage; fresh Sanaei readback proves the unchanged one-client baseline.

## Latest ramp checkpoint — stage 100 (2026-10-03)
- Read `docs/BULK_V3_100_ACCEPTANCE.json`: one 100-client batch passed create, Output, complete resource sampling and cleanup.
- Next: prepare stage 250 with durable rate<=100/sec and sufficient cleanup/sampling time; allowlist-only expansion is insufficient.
- Current canary accepts 10/25/50/100. Both gates and fleet generation remain closed. Runtime API/worker remain af1a541.

## Latest scale checkpoint — stage 10000 (2026-10-03)
- Read `docs/BULK_V3_10000_ACCEPTANCE.json` and the final section of `docs/DURABLE_BULK_V3_HANDOFF.md`. The 10000 configured-client stage is accepted and cleanup is complete.
- 50 existing clients were preserved; 9950 test clients reached fresh Sanaei/Output confirmation and durable deletion. 100 CREATE and 100 DELETE jobs are SUCCEEDED, all attempts=1; one committed DELETE was reconciled by explicit fresh read after a verification timeout. Original failure evidence remains.
- Production runtime: ae5ccdbe8d41a77078c12c371297bc6543b5899e, migration 135; subsequent evidence-only commits need no binary rebuild. Both execution gates and global fleet generation are disabled.
- Next: durable bulk-owned policy drift plus expiry/quota cleanup integration before fleet enablement. Do not replay earlier scale stages or start a new 10000 run as a bootstrap step. This test does not prove simultaneous traffic capacity.

## Durable owned lifecycle — accepted and continuously enabled (2026-10-03)

This checkpoint supersedes earlier statements that fleet generation must remain disabled. Read `docs/BULK_LIFECYCLE_ACCEPTANCE.json` and fresh production gates/scopes before any action.

- Runtime API/worker/canary: clean detached build b789a47639f5299e84093024262d799edaf644fb, migrations 136/137. Subsequent documentation commits do not require rebuilding identical product source.
- Durable policy updates map quota to totalGB, device limit to limitHwid, and lifetime to immutable ownership creation time plus configured seconds. Unknown v3 fields survive full-replacement UPDATE. Manual/non-owned clients are excluded.
- Fresh inactive/expiry/quota evidence creates owned-only BULK_DELETE plans. Refill waits for confirmed deletion and a new fresh deficit; durable rate allowance, immutable identities, PLANNED-before-POST ownership, global identity checks and read-before-retry recovery remain enforced.
- Full three-client cycles passed on panels f06917d6-8127-402b-9c3b-10e44d8e9039 and e1b309e2-6c67-45e0-925b-59f68293253d. Each: create, Output, policy change to 20 MiB / 75 seconds / HWID 3, actual expiry, delete three, three distinct replacements, Output, and owned-only cleanup restoring the exact baseline. All jobs succeeded at attempt=1. The second full run additionally asserted visible_until at least ten seconds before expiry.
- The first observer run crossed concurrent UPDATE reads and stopped. Fresh read and durable cleanup restored its baseline; original FAILED log/marker remain. Its cleanup-only SUCCEEDED record does not count as a full-cycle pass. e71b172 retries only inconsistent read observations during the policy wait, never a mutation.
- Fleet ramp 1 -> 2 -> 6 panels, delete chunks 10 -> 25 -> 50, removed 441 previously inactive owned clients in 15 first-attempt jobs. Nine manual-client hashes stayed identical. This deliberately changes the old 50-record baseline to one live manual client per panel; do not confuse that authorized cleanup with drift in the historical 10000 test.
- Three newly ready panels passed automatic v3 admission after normal Reality stability checks. At acceptance: 12 enabled scopes, each active_users=1, no scope errors, 12 fresh visible Output items, no unresolved lifecycle jobs or pending ownership. API/worker and all 12 target x-ui services active. Output remains API-derived, with aggregate refresh.
- Current global policy remains target=1, quota=0, lifetime=10800 seconds, device limit=0, rate=1/sec, ports=[443]; it is now enabled. No arbitrary higher capacity was selected.
- Continuous authorization: main gate enabled, kill_switch=false, concurrency=1; old bulk canary gate disabled, kill_switch=true, remaining_batches=0. Lifecycle control enabled, auto_enroll=true, cap=64 scopes, initial budget=10000 claims/scope, max chunk=100. Scope expiry is bounded by droplet expiry minus ten seconds and a maximum admission horizon. Existing closed/exhausted scopes never automatically rearm. An executor error closes the main gate; admission cannot reopen it.
- Panel 249a0885-6310-4103-81f4-0ab06071ea1f is intentionally unadmitted due to two historical FAILED CREATE rows. Do not clear or blindly retry them. Expired/disabled/unready panels are not rollout candidates.
- Structural inbound repair now freshly preserves all existing raw client records, unknown client fields and integer precision instead of posting the builder's bootstrap-only list.
- Focused real-PostgreSQL/race tests, full Go tests and clean detached tests passed. Fault tests cover committed UPDATE response loss, partial DELETE response loss, missing-only recovery, policy supersession, admission/gate races and no premature replacement. Actual production quota consumption was not generated: the quota-exhaustion trigger has real-PostgreSQL/HTTP-fault coverage, while production quota fields and actual expiry/replacement were verified.
- Evidence, original failures, staged hashes, health checks and deployment rollback binaries: /root/backups/dob-lifecycle-20261003. /tmp remains unsuitable for builds; use disk-backed GOTMPDIR.
- To stop execution, close the main and old bulk gates and disable lifecycle auto_enroll/global creation. Retain scopes, ownership and jobs for fresh-read reconciliation. Never run a second executor, reset audit rows, or replay completed canaries as recovery.
- Next operational work: observe finite-budget/expiry boundaries and independently reconcile the excluded historical panel if requested. The four requested lifecycle features are accepted for eligible v3 scopes; do not restart the earlier 10000 configured-client experiment.

## Latest Ads UDP / DNS continuation (2026-10-05)

Read docs/ADS_UDP_DNS_HANDOFF_20261005.md and docs/ADS_UDP_DNS_ACCEPTANCE_20261005.json before routing work. API/Worker deployed from clean source e5ec5e8beb5dd533885b252d2757ec1dba02a356. Latest user re-enabled UDP. Managed cached IPv4 DNS is server-direct and working over real Reality; Ads TCP/UDP remain residential and fail closed. Current upstream returns AUTH_REJECTED; slow panel API and cold tunnel latency remain separately unresolved. No AdMob display success or maximum-speed claim.
