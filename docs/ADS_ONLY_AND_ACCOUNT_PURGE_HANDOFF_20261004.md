# Account purge and ads-only residential acceptance — 2026-10-04

Continue from the canonical production tree, not GitHub main:
- Control server: serverprojects.ptr.network
- Tree: /root/projects/Digital-ocean-bot-canonical-e2e
- Continuation branch: checkpoint/final-e2e-20260929
- Runtime source: add8e110cfcb3efdb72ebbc47fe9469a5c22693d
- Implementation commits: 75c5b110e65b3bd1e1680473de92aaaa507f4ea2 and add8e110cfcb3efdb72ebbc47fe9469a5c22693d
- Acceptance: docs/ADS_ONLY_AND_ACCOUNT_PURGE_ACCEPTANCE_20261004.json
- Evidence: /root/backups/dob-delete-ads-20261004

The subsequent documentation checkpoint intentionally advances HEAD beyond the runtime source without changing binaries. Verify clean HEAD == origin/checkpoint/final-e2e-20260929 before editing. Both deployed binaries were built from a clean detached worktree and report vcs.modified=false.

## Current verified production state

38 active READY servers, zero broken/inactive/panel-attention servers; 38 fresh APPLIED routing proofs. Output has 76 fresh distinct identities: 38 DIRECT and 38 RESIDENTIAL, with no overlap. Capacity is 76 active / 76 target, deficit zero and no errors. Four live accounts (two DigitalOcean, two Vultr) are healthy. No local account/server deletions remain pending.

Global Reality is enabled, revision 23, target 2 per panel, creation rate 1/sec, lifetime 10800 seconds, quota unlimited, both classes enabled. The normal durable client mutation gate remains enabled, kill_switch=false, concurrency=1, global scope for ongoing lifecycle work. The legacy bulk gate remains closed (enabled=false, kill_switch=true). There are 39 enabled lifecycle scopes, of which 38 correspond to eligible serving panels.

API and worker are active. No schema migration was added; current migration remains 000146_panel_network_access.

## Account deletion

Normal DELETE retains provider-first durable cleanup and verifies managed resources absent before removing the saved account. The shared transactional purger now handles ownership, lifecycle/scale rows, scoped gates, routing allowlists and account-owned dependencies in the required order. Scoped gates are closed and cleared before FK scope removal. Account/provider/panel locks prevent competing work; injected failure tests prove transaction rollback.

For disabled blocked accounts with an existing deletion intent older than two minutes, an explicit admin local-purge action is available. It requires the exact fresh raw provider state, distinct remaining-resource count and acknowledgement that cloud deletion is unverified. The raw deletion provider_state must not be confused with the DISABLED display status. Replaying a successfully completed purge is idempotent.

Four previously requested blocked accounts were purged using the actual browser Delete action. Zero linked rows were independently verified across 32 account_id tables, with other live accounts unchanged and no test fixture residue.

IMPORTANT: provider deletion could not be verified for 20 previously tracked resources of those four accounts because provider access was blocked. Local purge does not establish that those cloud resources were deleted. The statement of removal covers operational account data/credentials/jobs/statistics, not secure erasure of old backups or system journals. Retain this distinction in any subsequent report.

## Residential routing

RESIDENTIAL identities now send only advertising category matches through residential. Non-advertising and DNS traffic are direct. DIRECT identities remain direct and have separate Output identities.

Verified installed asset rules:
- geosite:category-ads-all
- geosite:category-ads
- geosite:google@ads
- geosite:facebook@ads

The user-facing Google ads/Ads labels map to google@ads/category-ads. geosite:google-ads and geosite:ads are not valid aliases in the installed asset. Do not substitute entire Google or Facebook categories.

Ad rules cover TCP and UDP. HTTP/TLS/QUIC sniffing is freshly validated before APPLIED state. Exported client URIs use xtls-rprx-vision-udp443 while server client flow remains xtls-rprx-vision. Matched ads fail closed when a configured proxy is unavailable; unrelated traffic stays direct. With no residential configured, traffic is direct. Keep existing destination-IP security guards.

The temporary DOB_RESIDENTIAL_ADS_ONLY_PANELS canary override and its systemd drop-in have been removed. The default new policy is applied and freshly proven on all 38 serving panels.

Domain lists require an available or sniffable hostname; opaque/ECH/unrecognized direct-IP advertising cannot be universally identified using these rules.

## Actual traffic acceptance and limitation

The video was reviewed across its complete 190.46-second visual sequence. The old all-traffic residential route, failed UDP DNS and Vision UDP443 restriction were reproduced before correction.

On the scoped production canary, both identity classes now passed:
- Non-ad HTTPS egress through the server IP.
- Real UDP DNS resolution.
- QUIC TLS handshake with ALPN h3 to Google.
- Advertising HTTPS connectivity (application returned HTTP 404).
- Running-core TCP/UDP route proofs distinguishing ad and non-ad domains.

Res1 authenticated TCP, but a real SOCKS5 UDP ASSOCIATE request returned reply 1. Therefore advertising UDP/QUIC through this residential endpoint is NOT accepted as working. Permitting UDP in local routing cannot make an upstream UDP relay available. A UDP-capable upstream and a fresh end-to-end advertising UDP/QUIC test are required to close this item. Non-ad UDP/QUIC direct is already proven.

The requested residential IP speed/health selection strategy is deliberately deferred.

## Memory and regression checks

A legacy 453MiB host encountered Xray OOM during rollout and recovered through the existing bounded repair path. All 38 hosts were then inspected through the existing pinned SSH infrastructure. One remaining small no-swap host received the committed journaled memory guard, serially scoped to its exact panel. The final survey found zero hosts requiring the configured headroom repair; all three small hosts have managed swap. No broad restart or blind mutation retry was used.

Focused PostgreSQL/unit tests, race tests, full Go tests and clean detached full tests passed. Fault/race coverage includes purge rollback, provider lock serialization, stale requests, lost-response replay and routing recovery. Actual mobile browser deletion, four real local purges, 32-table checks, real VLESS TCP/DNS/QUIC and all 38 routing/memory observations were completed.

Preserve the two historical CREATE failures for audit. Do not retry or rewrite them based only on their FAILED state.

## Next work

1. Resolve the current residential endpoint's rejected UDP association, then perform scoped end-to-end advertising UDP/QUIC acceptance.
2. Verify the 20 unconfirmed external resources through authorized provider access; do not infer deletion from their absence in the local DB.
3. Separately perform the pending scoped real quota-exhaustion acceptance from the prior handoff.
4. Design and benchmark residential IP selection only when the user starts that deferred task.

Use fresh production evidence before any follow-up. Never reopen mutation gates, recreate deleted account fixtures, retry completed purges, or repeat provider mutations based only on historical logs.
