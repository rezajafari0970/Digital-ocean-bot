# Reversible residential performance profiles — 2026-10-05

Runtime source: 9e98182911beeb4c2b8b4598b350e4c8cc8e9130. Additive migration 151. Evidence: /root/backups/dob-residential-performance-20261005.

## Product
Residential → Performance & rollback provides a two-step preview and Start timed test on one eligible, freshly verified, sufficiently long-lived server. The immutable profile can be extended to selected servers only after every existing target has a fresh running proof. Keep disables its deadline after the same verification gate. Rollback remains available for a kept profile. Finish rollback before starting another profile.

Native controls: fast share 50–90%, fast group count, probe interval 10–60s, timeout/maxRTT, per-proxy leastLoad costs, direct cached IPv4 DNS via TCP or DoH, IPv4 proxy-gateway resolution, managed outbound TCP Fast Open/keepalive/user timeout, optional AsIs routing strategy, level-0 buffer size. Defaults are 80%, 3 fast paths, 10s / 3000ms, direct TCP DNS, gateway IPv4, TCP options with Fast Open off, 30s/20s keepalive and 20000ms TCP user timeout, preserved routing strategy, 64 KiB buffer.

Destination targetStrategy ForceIPv4 was rejected from the product after installed Xray 26.3.27 accepted the JSON but a real SOCKS fixture still observed domain targets. No ineffective destination-IPv4 control is exposed. No passive failure observer, hard concurrency cap, provider UDP certification, client Mux/MTU tuning, or Android AdMob timeout change is claimed.

## Durable recovery
- One global open/kept profile. Serialized admin operations, operation UUID/request hashes, optimistic experiment versions, per-panel generations.
- Persist baseline and pending owned values before remote mutation; response loss reconciles current observations.
- A worker checks deadlines every five seconds; three failed verification attempts during a running trial also request rollback. Worker downtime delays processing until restart.
- Rollback regenerates managed routes from CURRENT endpoints/clients; it does not restore a stale whole-panel or client snapshot.
- Presence-aware snapshots restore the global routing strategy and buffer fields. Unexpected external changes report a conflict rather than overwriting them.
- Server restoration and routing verification commit together with generation guards. Offline/unverified servers remain ROLLBACK_PENDING; actually deleted servers are explicitly RETIRED.
- Closing routing execution gates pauses reconciliation and is shown in the UI. Reopening the gate resumes queued rollback; the feature does not override a closed operator gate.
- Runtime template save/reload is serialized under the existing panel config lock. Existing connections can be interrupted when Xray actually reloads.

## Acceptance
OpenAI API plan and Development Orchestrator PLAN→IMPLEMENT→TEST→VERIFY→CHECKPOINT completed. Full Go suite, PostgreSQL/race tests, real installed-core TCP/UDP failure/distribution/recovery tests, applied-profile plus rollback core tests, and mobile/desktop browser tests passed. Browser fault injection dropped a response after commit, retried with the same operation identity, refreshed the page and requested rollback; no duplicate experiment was created.

Live canary: 0ae83b7e-97a6-4a22-abbf-715619b65a6d (192.248.165.92:2053).
Experiment b2718145-8cb5-4c35-bb93-a4e497149589.
- Start 16:22:04Z; applied generation 1 freshly verified 16:22:15Z.
- Independent matrix 38/38 passed 16:22:46Z.
- Explicit API rollback requested; generation 2 RESTORED, experiment ROLLED_BACK, profile null, verified 16:23:15Z.
- Independent restored matrix 38/38 passed 16:23:37Z.
- Final state: no active tuning profile; previous settings remain active. The user can start their own timed test from the deployed UI.
- Actual production UI read-only mobile/desktop acceptance passed.
- Reality profiles and client mutation gate byte-identical; all 12 endpoints in the predeployment snapshot retained. No proxy import/delete or client lifecycle gate change by this task.

At the final metadata snapshot all 24 currently eligible READY servers had fresh controller routing proofs at revision 598. This is not a new independent fleet test and does not erase historical six-server timeouts from the preceding task; fleet membership changed through normal lifecycle. Only this task's one canary received independent before/after-profile acceptance. Actual app ad load latency and upstream provider UDP have not been benchmarked here.

## Operational rollback
For a performance test use the panel Rollback button, and wait for ROLLED_BACK / RESTORED. A queued request alone is not a successful restoration.
Before reverting these binaries to pre-151 code, first roll back every non-null profile using this version and verify restoration. Keep additive migration 151 while previous binaries run. Then restore api.before, worker.before and static.before from the evidence directory. Do not remove the worker that owns pending restoration. Deployment fallback itself ran before any experiment could be started; it restores previous binaries/static on readiness failure.

Do not replay old bulk imports, lifecycle scale experiments, reboots, migrations, or completed canaries. Next work is the user's measured app test of one timed profile; tune one group of variables at a time and keep only after their ad-load and connection results are acceptable.
