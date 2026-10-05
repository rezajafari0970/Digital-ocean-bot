# Residential recovery — 2026-10-05

Canonical continuation branch remains checkpoint/final-e2e-20260929. Verify live git/DB/runtime before further work. This checkpoint supersedes the UDP-unaccepted status from 2026-10-04, with the limits below.

## Runtime

API: 1eb5c786a537da42952049a57ec5916ed6df24db.
Worker: 11f97fa235b93e0aff9f75a0e3f86549eeb2ce85.
Both are clean detached builds. The last change only required restarting Worker. API/worker active, no automatic restart observed.

Main mutation gate restored enabled, kill_switch=false, concurrency1, no panel scope. Old bulk gate remains closed. Worker remains the only client mutation executor. No new failed mutation jobs since accepted fleet recovery.

## Root causes and fixes

The gate stopped overnight on disagreement between the inbound and global client projections. This is client field disagreement, not the Global Reality form. Fresh observations agreed when reviewed. Commit917c900 adds at most three complete fresh dual reads with100/200ms cancellable backoff for this specific error. Persistent conflict still fails closed.

An old queued CREATE had already expired when execution resumed. Read-before-write proved it absent; ownership became ABORTED and the immutable job became OBSOLETE with audit preserved. Commit1eb5c78 retires such lifecycle plans after complete fresh reconciliation and before POST. Already-observed members remain ACTIVE; absent planned members abort. Identity conflicts remain errors. Historical unrelated CREATE failures were untouched.

The existing Residential sticky endpoint did not forward UDP. An isolated session selecting a UDP-capable IP, on an available port within the existing account, passed TCP, real UDP DNS and HTTP3. Both existing Residential records were updated to that verified account setting, first scoped to one panel, then fleet-wide. This is per-record provider configuration, not a provider-specific production-code dependency. Account proxies were not modified. Never infer UDP support from the HTTP health badge alone.

Exact installed Xray protocol.go confirms its handshake sends zero source address/port for UDP association, perRFC1928. An early raw probe incorrectly supplied the destination, and an early raw QUIC relay bound loopback. Those diagnostics were corrected; the accepted evidence is native Xray plus real VLESS/Reality end-to-end traffic. Do not modify Xray based on the discarded probe assumptions.

One serial serving routing lane could take139seconds to revisit all38 panels, exceeding the unchanged60-second Output proof TTL. Commit11f97fa splits serving panels across eight disjoint bounded lanes; retired panels retain a separate serial lane. Per-panel config locks and mutation locks are preserved, and client mutation concurrency remains1.

## Acceptance

All38 serving panels passed saved settings and running route proofs across the window, including the three-provider DNS pool, UDP-selected Residential endpoint, BrowserLeaks, ordinary domains, TCP and UDP, and both route classes. Four panels changing plans during the first sweep passed fresh rechecks.

At05:54:56UTC, Output published38Direct and38Residential unique configs, plain URI lines, with no shared identity across the links. The dashboard reported38active servers, zero broken/inactive servers, four healthy accounts and one Worker. Output counts can still dip while a credential expires and its replacement is verified; that is deliberate withholding, not stale publication.

Real tunnel tests passed ordinary traffic with server egress, BrowserLeaks and opaque traffic with Residential egress, DNSA/AAAA/HTTPS65 over TCP and UDP client inputs, and real HTTP3 against adservice.google.com. Its404 is transport acceptance, not proof that an advertisement played or earned anything. TLS sniffing was tested with an IP destination supplied by SOCKS.

Focused/race/full/clean-build tests passed, including real-core fail-closed DNS tests, expired-plan partial recovery, persistent identity conflict, and concurrent disjoint routing lanes. Six-minute monitoring covered continuing expiry/deletion/replacement without another mutation gate failure; inspect the JSON for sampled proof ages and final state.

## Preserved settings and next work

Direct lifetime0; Residential lifetime240seconds; both creation intervals60seconds. A six-minute creation interval is360seconds in the independent profile UI. It is distinct from lifetime and only creates while below target. Do not silently change either user's saved value.

Both Residential records currently use one upstream account/session; they are not independent provider capacity.30k concurrent users and sustained UDP availability remain untested. The next capacity/capability project should measure per-endpoint UDP, HTTP3, available threads and bandwidth with provider-neutral admission and independent failure domains. Do not relax leak protection, Output freshness, or the Worker fail-close invariant.

Private evidence: /root/backups/dob-residential-quic-20261005. Do not print credentials, raw config URIs, endpoint usernames, or private before-state artifacts.
