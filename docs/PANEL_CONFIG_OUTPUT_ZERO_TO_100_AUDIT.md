# Panel / Sanaei / Reality / Config / Output Control Plane — Zero-to-100 Audit

Date: 2026-10-02

## Authority and data path
Post-install panel state is durable in PostgreSQL. Sanaei API is the runtime authority for inbound/client inventory and Output generation. SSH is retained only for installation/bootstrap/host-level repair and Reality target discovery where the panel API does not provide the required host capability; Output never reads configs over SSH.

The active flow is:
PANEL_COMPLETE -> eligible panel -> Reality target stability -> Reality credential registry -> VLESS/TCP/Reality inbound -> user capacity reconciliation -> Sanaei API snapshot -> output_config_snapshots -> Output/share response.

## Findings
1. Reality policy had been intentionally disabled by the capacity cleanup endpoint. Cleanup disables the policy before deleting clients to prevent immediate rebuild.
2. Output endpoint previously depended on materialized snapshots without forcing a live Sanaei refresh. With zero snapshots, Output was empty even when panels were eligible.
3. Dead in-memory Output cache infrastructure still allowed a future stale 24-hour fallback to be reintroduced.
4. Initial live audit found eligible panels with historical structural/capacity metadata but Sanaei API raw inventory empty.
5. User-capacity mutation read the global policy independently and originally did not share the staged rollout gate with global Reality reconciliation.
6. A fixed panel allowlist would block future newly provisioned panels forever after promotion.

## Corrections
- Output and share requests now synchronously refresh eligible panels from Sanaei API before reading output_config_snapshots.
- Refresh failure no longer falls back to 24-hour in-memory config data.
- Dead OutputCache/async stale-cache code and Server fields were removed.
- Migration 000111 adds reality_rollout_panels for explicit canary membership.
- Global Reality and user-capacity mutations share the same rollout authority.
- Migration 000112 adds reality_rollout_control with canary/stable modes.
- canary mode mutates only explicitly allowed panels.
- stable mode automatically reconciles all eligible current and future panels.
- Existing 1-second Output warmer remains a near-real-time materialized-view refresher; session snapshots are invalidated before Sanaei API reads, so it is not a config data cache.

## Canary and rollout evidence
Policy during verified rollout:
- port: 443
- protocol/transport/security: VLESS / TCP / Reality
- target users per inbound: 1
- user lifetime: 10800 seconds
- quota: 0 (unlimited)
- device limit: 0 (unlimited)
- SNI mode: scored

A reachable Vultr panel was selected as canary. Canary result:
- Reality inbound 443 present/enabled
- active users 1
- deficit 0
- fresh Output snapshot 1
- visible_until present and bounded by both client expiry and droplet expiry minus 10 seconds

After canary success, rollout was promoted. Seven then-current eligible panels all showed Reality443=1, active_users=1, deficit=0 and fresh_output=1.

Rollout mode was then promoted to stable. A subsequently eligible/new panel entered Reality stability warming automatically, accumulated observations, selected a stable SNI, created Reality443, reached active user capacity and produced fresh Output without manual intervention.

## Final live invariants
At final verification:
- eligible panels: 8
- eligible panels without SNI selection: 0
- fresh Output configs: 8
- expired-but-visible configs: 0
- droplet expiry minus-10-second violations: 0
- stale output snapshot rows older than 5 minutes: 0
- production rollout mode: stable

## Failure/self-healing
Sanaei runtime uses per-panel circuit protection and session invalidation. Reality SNI selection requires stable history rather than immediately committing a single scan. Observed new-panel warming converged automatically from ErrRealityWarming to a selected target, inbound, client and Output.

Panel/API failures do not cause stale configs to be served: a failed refresh does not refresh last_seen_at, and Output queries require snapshots newer than 15 seconds and visible_until > now. Ineligible panels are purged by the 1-second materialized-view refresher.

## Verification
- race tests: adminapi and panels packages PASS
- full go test ./... PASS
- migrations 000111 and 000112 validated against production schema before promotion
- production API and worker active with matching runtime/manifest hashes after each promotion
- live Sanaei API inventory generated valid Reality configs on all eligible panels
