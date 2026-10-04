# Residential routing, DNS and validation — 2026-10-04

Continue on serverprojects.ptr.network from /root/projects/Digital-ocean-bot-canonical-e2e, origin/checkpoint/final-e2e-20260929. Verify clean canonical HEAD and origin before edits. Production is authoritative; do not re-prime, re-run scale experiments or merge main.

## Runtime and final evidence

- API: be68fa76e78e6e6eb07c61e0951ba0711065b228, clean detached build.
- Worker: 91b97ad66fd41d27b0789429d19d398f2ae78738, clean detached build.
- Migration: 000147_residential_health_observation.
- The documentation checkpoint intentionally advances source HEAD past runtime revisions.
- Acceptance: docs/RESIDENTIAL_DNS_VALIDATION_ACCEPTANCE_20261004.json.
- Evidence directory: /root/backups/dob-routing-dns-validation-20261004.

Final captured DB: 38 APPLIED serving panels, all proofs younger than 60 seconds, zero serving residential_sync failures, routing revision 504. Independent panel API/core verification passed all 38 panels and 152 protected-route probes at 14:04:05 UTC. Both systemd services active/running, auto-restart counts zero.

Actual mobile browser acceptance passed the dashboard, compact per-account billing, proxy/residential isolation, config validation, DNS catalog, plain live Output, and token-bound Output classes. Dashboard: 38 active servers, zero inactive/broken/pending-deletion servers, four healthy accounts (two DigitalOcean/two Vultr). Invalid edits preserved Global Reality revision 23.

Important availability result: Res1 was down at final capture. DIRECT Output remained nonempty; RESIDENTIAL Output was correctly empty. Do not describe this as 76 currently available configurations or a permanently repaired upstream. A transient read-only route-test timeout and a transient worker failure recovered on fresh reads; neither caused a blind save/restart retry. These observations remain in the evidence logs.

## Changes and invariants

Commits:
- 38b60c895512095f3e0133579e1ccfd0f8579c4b: routing protection, DNS catalog, validation.
- be68fa76e78e6e6eb07c61e0951ba0711065b228: apply an actual blocking plan when sniffing is unverified.
- 9b12907851144cc6493cbeaf611922ea7a40036f: wait for Xray route API readiness using reads, without duplicate restart.
- 91b97ad66fd41d27b0789429d19d398f2ae78738: separate endpoint health observations from routing intent revisions.

Hardened routing is enabled fleet-wide. No canary systemd drop-in remains. For RESIDENTIAL identities, listed advertising domains use residential for TCP/UDP; client DNS/opaque traffic/literal IP destinations cannot fall through to the server's direct outbound. Known non-ad domains continue to use direct as requested. DIRECT identities remain independent. HTTP upstream UDP is blocked. Missing/removed/disabled/unverified residential endpoints block protected traffic. Unobserved inbounds and the default outbound are blocked.

Sniffing must be enabled for HTTP/TLS/QUIC, with metadataOnly=false and no excluded domain/IP bypass lists. Invalid sniffing applies a blocking residential plan rather than merely hiding Output while already-shared clients retain unsafe routes. Existing valid settings were preserved; this task does not claim a measured sniffing latency improvement.

Do not claim universal advertising classification. ECH can expose only an outer hostname; unlisted/shared advertising domains can appear non-advertising. Ads-only splitting with direct non-ad traffic cannot guarantee that every possible advertising connection is recognized. The optional strict-versus-selective question had no response, so the established ads-only requirement was preserved. A strict no-server-egress guarantee for all potentially advertising traffic requires all traffic of the residential subscription to use residential or be blocked. This policy change was not silently enabled.

## DNS

The supplied list is deduplicated into 123 catalog entries in internal/panels/resolverpolicy/catalog.json, exposed through authenticated GET /api/v1/configs/dns and the Global Reality form.

Control-server UDP/53 observations: 65 public IPv4 entries answered both example.com and adservice.google.com; 23 were not verified; 31 IPv6 entries are disabled by the existing server IPv6 policy; four addresses are private 10.202.*. These are dated observations, not universal resolver reliability claims. Filtered/family profiles remain labeled and inactive.

Active bounded pool, verified through the real canary as well:
- 1.1.1.1
- 8.8.4.4
- 1.0.0.1

All managed Xray templates now contain this pool, UseIPv4, caching, disableFallbackIfMatch=true and tag dob-route-dns-query. Built-in queries use normal protected routing, with no localhost/local-transport fallback. Do not blindly enable all catalog addresses: some are private, filtering, unresponsive or IPv6-incompatible with current policy. Do not imply that all encrypted DNS hostnames are recognizable.

## Global config edit

The screenshot's rate 600 exceeds the existing durable 1–100 users/second limit. Both route classes with an enabled target of one are also invalid: two classes require a target of at least two.

The API now reports the precise invalid field/detail and the mobile form validates the same constraints. Browser coverage accounts for the site's numeric-input normalization to text inputs. Valid range limits remain aligned with durable worker/database constraints; they were not increased without scale evidence.

The normal saved policy remains enabled, revision 23, target two, rate one/second, lifetime 10800 seconds, unlimited quota, both classes. Main client mutation gate remains intentionally enabled, kill_switch=false, concurrency=1. Legacy bulk gate remains closed. Historical CREATE failures were preserved.

## Root causes of fleet instability

1. The panel can report Xray running before its gRPC route API on 127.0.0.1:62789 is ready. Explicit RPC Unavailable is now classified and polled for up to eight seconds. Reads only; no additional save or restart for this readiness condition. Actual route mismatches retain the bounded restart path. Lost-response read-before-write recovery remains intact.

2. The old residential_route_update trigger bumped the entire fleet routing revision on every healthy/down transition. Res1 flaps therefore repeatedly rebuilt/restarted every Xray and invalidated both Output classes.

Migration 147 removes health status from that intent trigger. Endpoint edits, disable/remove, and credential changes still invalidate routing. A previously verified endpoint with unchanged configuration remains the protected egress during a transient failed probe. Its transport failure cannot turn into direct fallback. Output independently requires current health and fresh proofs, so residential subscriptions disappear immediately when unhealthy; direct subscriptions do not disappear solely because a residential probe failed.

Eligibility for retaining an endpoint requires last_success_at >= updated_at and enabled=true. Healthy alternatives are preferred. No successful observation for the edited endpoint means protected traffic blocks. This is not the future multi-IP optimization strategy requested by the user.

Postgres integration tests verify repeated health flips do not increment revision, direct Output remains available, residential Output hides/reappears with health, worker reconciliation does not save/restart solely for the same endpoint's temporary failure, and removal still blocks/reloads.

## Traffic, fault and test evidence

- Focused, race and full Go tests passed with the existing isolated test database.
- Clean detached full tests and binary provenance passed.
- Installed Xray tests cover real TCP/UDP sinks and unavailable upstreams: protected traffic cannot fall through to direct.
- Unit/fault tests cover malformed/lost responses, invalid sniffing, opaque IPs, default/new inbounds, DNS fallback, route-API readiness and concurrency.
- Real VLESS canary 69e68a2a-8a42-4c9f-9f12-302920d729bc passed TCP, UDP DNS to all three active resolvers, and actual HTTP/3/QUIC, including an advertising HTTP/3 response. Protected opaque HTTPS exited from a non-server IP; known non-ad HTTPS remained direct.
- A real upstream-outage canary earlier in this rollout confirmed protected traffic blocked with direct positive controls.
- The final health-retention refinement was tested separately against installed Xray and final fleet running-core proofs; do not label the earlier blocking-mode traffic capture as a fresh v4 traffic capture.
- Final read-only fleet evidence: fleet-final-recovery.log. The initial transient timeout is retained in fleet-final.log.
- Final mobile/API evidence: final-ui-v4.log.
- Final DB snapshot: final-production-snapshot.json.

## Res1 and deferred work

The old sticky session became unreliable. One scoped candidate was tested with HTTPS, UDP and HTTP/3 before updating the existing residential record through its authenticated admin API. The tested username suffix is now ;udp=true;session=dobdns20261004. Password unchanged; the account-network proxy was independently verified unchanged. Unknown-result handling used fresh reads.

This restored traffic during the healthy canary but did not establish stable upstream availability. The final observation is down. Do not rotate more sessions blindly, mark health successful manually, or weaken Output freshness to conceal this. Residential IP health/speed selection remains the deferred strategy work. The current monitor is an endpoint connectivity check, not a guarantee of UDP or every destination.

The separate historical cloud-cleanup task is unchanged: 20 previously associated resources remain unverified because three DigitalOcean accounts were locked (422) and one Vultr credential returned 401. No provider DELETE was sent in this task, and no cloud/backups erasure is claimed. Preserve the recovery evidence from docs/PLAIN_OUTPUT_UDP_CLOUD_HANDOFF_20261004.md.

Next logical work: agree on the strict residential no-server-egress policy if an absolute guarantee is required, then design and test upstream health/IP selection using actual TCP/UDP/QUIC evidence. Do not confuse that external availability work with this accepted routing/validation/DNS patch.
