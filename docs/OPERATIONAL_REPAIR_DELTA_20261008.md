# Account, panel and proxy repair continuation — 2026-10-08

This delta supersedes the October 3 frontier and archived browser-console work. Always inspect current canonical HEAD, runtime manifest and live database first; another authorized workstream can publish concurrently.

## Source and evidence

Canonical: `/root/projects/Digital-ocean-bot-canonical-e2e` on `serverprojects.ptr.network`. Runtime: `/opt/digital-ocean-bot`. The current release baseline is `ae0593554048249b5ba1cdfe9e4405fa4b51e5b8`; existing runtime source is `d02f0450e2e8b71e5ff4b28ed52cb03eefe2f59e`, with only newer documentation in canonical. Preserve the residential allowlist, native-verifier 30-second bounded readiness fix, reports and drop-in. The prior 7cc709e integration passed all tests but was not deployed because actual API verification returned 429 credit_balance_exhausted. Credits are restored and a new exact integrated review is required.

- Deployed parent `a41047c0af17c9a9f164d08b05f29957add170d4`: guardian packaging; account deletion UI/error handling; scoped purge/telemetry; DigitalOcean native status message.
- Independently approved source `38c3b6df529cbac8e1a7b89d38e1e5c645c222da`: account legacy-browser artifact cleanup.
- Independently approved source `00bec91d3d763583c43a50b8f0b52158a91d7856`: idempotent guardian systemd transitions.
- This integration carries those exact changes plus bounded idle provider polling. Its commit is the checkpoint containing this file. Deployment is not inferred from source presence.

Current job: `docs/development-jobs/operational-release-20261009.json`. Evidence and latest operational status: `/root/backups/dob-operational-release-20261009/`. Previous blocked attempt: `/root/backups/dob-operational-integration-20261008/` (preserved exact tested digest 99097625ee747e09ec74b1ec1a8dcbc8fcb401700c4ed869aa0c235316975d2f). Read `status.json`, `commit.txt`, `reviewed-evidence/api-review.json`, `deploy.exit`, and final runtime/traffic evidence when present. Missing evidence means that phase is not verified. Earlier jobs have corresponding `dob-account-panel-repair-20261008`, `dob-account-purge-artifacts-20261008`, `dob-guardian-reconcile-20261008`, and `dob-proxy-economy-20261008` evidence directories.

## Changes and invariants

1. Account deletion: a blocked cloud deletion exposes **Delete from system…** near the top of the account card. Explicit confirmation acknowledges unverified provider cleanup. Error responses remain errors while the account still exists; lost-response success requires a fresh read proving absence. Account purge removes its operational database data, credentials, scoped telemetry and legacy browser profile. Canonical UUID validation precedes derived locks; filesystem deletion is anchored to the fixed directory with no symlink traversal. Native in-flight/checkpoint fences remain mandatory. Profile removal can precede a failed database commit; retry is idempotent. Historical backups/logs are not secure-erased.
2. Invalid provider credentials cannot prove cloud servers were deleted. Never delete real accounts merely to test the button. User-requested individual local purge and verified provider cleanup have different external effects; neither fabricates the other.
3. Guardian deployment: stage and verify both Linux architectures, hashes and ELF architecture before publishing. The previous release omitted these files, producing panel protection failures. Routine reconcile avoids repeated `systemctl enable/disable`, still starts/stops as needed, and reloads/restarts on owned unit/binary changes. Ownership, hashes, revisions, allowlists and fresh receipts are unchanged.
4. DigitalOcean `warning` is explained by the provider's bounded native status message. Observed accounts had exhausted their Droplet allowance. This is not repaired by hiding warning or bypassing capacity; it may clear when capacity is released. No quota purchase/change was performed.
5. Idle provider inventory may refresh every five minutes only when there is no pending operation, deployment, backfill or recovery and no unknown/near expiry. A satisfied desired count can qualify with null/past next-build time. Disabled/deleting/missing accounts and lookup failure cannot qualify. Existing pre-mutation freshness and account-proxy fail-close remain in force; increasing desired count makes the next check active again.

## Proxy economy and remaining operational limits

Account/provider API, account identity, and account DNS use the dedicated account proxy. Sanaei management stays direct IPv4 and ignores environment proxies. Residential subscription egress is a separate role; retain its explicitly authorized destination allowlist and current deployment selector. Do not disable residential routing to improve account-proxy measurements.

Economy mode is enabled globally. Earlier comparable 900-second, eight-account wire sampling measured approximately 85% reduction; **90% remains unverified**. A later passive sample measured 3,727,097 bytes over about 900 seconds, but cohort and load changed; it is not a new comparable percentage or provider billing proof. TLS resumption keeps credentials/identity/generation scopes separate. Rejected-token retries are paced and valid credential repair rearms recovery.

Panel-attention counts fluctuate with server lifecycle and live CPU/connection-table pressure. Do not disable protection or relax thresholds to force zero. After correcting missing guardians, native receipts became APPLIED on more than twenty nodes; remaining CPU/conntrack alerts require current evidence. PID 1 sampled on three nodes before integration used roughly 0–1.6% of one CPU over 64 seconds, so earlier lifetime `%CPU` listings do not prove that systemd caused sustained pressure. Never claim measured CPU improvement without a valid same-node interval comparison.

At the last pre-integration review several Vultr credentials remained invalid and 73 tracked servers awaited cleanup. These are historical observations, not fixed counters. Check live counts and user account changes. Recovery checkpoints must not be erased to unblock purge or deployment.

## Resume and publish

Use the actual development orchestrator PLAN → IMPLEMENT → TEST → VERIFY (actual server OpenAI API) → CHECKPOINT. Preserve failed-test/review evidence. Full Go, relevant race, Chromium deletion, guardian packaging fault tests and residential installed-core checks bind to the same source digest. Publish only a clean detached build; acquire the shared deployment lock before rechecking canonical/runtime and advancing them. Preserve the residential drop-in byte for byte and keep the retired Vultr browser manager disabled. Verify all three runtime process hashes, guardian files and readiness after publication, then save native panel/traffic observations and report remaining external issues honestly.
