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
