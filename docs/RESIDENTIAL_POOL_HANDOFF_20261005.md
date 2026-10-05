# Residential bulk management and adaptive pool — 2026-10-05

## Deployed product

API, worker and static UI were deployed from clean source `7d41d441ff6be4077bdd72f30de65b1739714abe`; migration `000150_residential_pool` is applied. Product commits are `3351972` and `7d41d44`. Subsequent evidence-only commits do not require rebuilding identical product source. Read `RESIDENTIAL_POOL_ACCEPTANCE_20261005.json` and fresh database state for current evidence.

Residential → **Bulk add SOCKS** accepts 1–100 `host:port:user:password` lines, ignoring blank lines. Prefix defaults to `ads`; numbering skips existing names. Every generated name is editable before submission. Display spelling/case is preserved; database uniqueness rejects names equal after case folding, including concurrent writes. Names have an 80-character limit. IPv4, domain and bracketed IPv6 endpoints are supported; password colons are retained.

Imports are atomic and use a durable request UUID plus payload hash, so retrying a lost response cannot duplicate a committed batch. Credentials remain encrypted in the existing secret store; list/error responses redact them. Copy all/selected explicitly exports the selected IDs for an authenticated administrator with no-store responses. Clipboard fallback is an ephemeral dialog. Delete all/selected shows a count/name confirmation and deletes only reviewed residential IDs; shared account API proxies are retained.

## Selection, failure and recovery

Each serving Xray measures its own configured proxy paths with burstObservatory HEAD requests to Google's 204 connectivity endpoint. Interval is 10 seconds with native randomized scheduling, sampling=1, timeout=3 seconds. Unobserved/failed paths are excluded. A subsequent successful local probe restores eligibility; this does not repair invalid credentials or a broken provider endpoint. Failure detection is periodic, not instantaneous, and existing connections are not migrated.

Two routing stages distribute new connections: four of five random loopback lanes select the fastest healthy group (target size ceil(configured candidates / 3)); one lane selects among all healthy candidates. Native leastLoad uses latency with maxRTT=3 seconds. Thus about 80% of new flows enter the fast group and 20% explore all eligible paths. The fast group is also eligible in the latter 20%. This is not an exact byte/throughput split, and if only one path is healthy it necessarily receives all eligible connections.

TCP pools include supported proxy types; UDP pools include SOCKS only. TCP connectivity checks do not independently prove a provider supports UDP. Selection measures response time, not available bandwidth. If no eligible path exists, residential traffic is blocked; it never falls back to direct egress.

The control-server monitor separately updates the panel's health, sanitized error, timestamps and EWMA fields. It uses 16 bounded workers, 5-second checks, approximately 30-second healthy and 10-second down rechecks. Stale observations cannot overwrite newer endpoint versions or observations. Probe results do not bump routing revision or restart Xray; structural imports/deletes/credential changes do trigger reconciliation. Pool routing is enabled fleet-wide with no temporary panel allowlist, including future eligible servers.

Exact scope remains `geosite:google@ads` plus temporary `domain:browserleaks.com`. Other traffic and cached IPv4 DNS remain direct. Client profiles, creation policies and mutation execution gates were not changed by this deployment.

## Validation and rollout

- Full Go suite, focused race suite and isolated PostgreSQL integration tests passed. Coverage includes atomic rollback, case preservation/uniqueness, concurrent idempotent import retries, encrypted credentials, admin export/no-store, deletion scope and health recovery without routing-revision churn.
- Real headless desktop/mobile browser tests passed bulk preview/edit, exact names, copy all/selected, selected/all deletion and confirmations. Production admin read-only acceptance and share-class boundaries passed.
- Installed real Xray fault test passed unknown/down exclusion, withdrawal, recovery and all-down blocking. A healthy fast/slow pair received 108/12 of 120 TCP connections and 108/12 UDP flows; recovery received 90/10 of 100 connections. UDP support was exercised against the test SOCKS fixture, not certified for every live provider.
- First production canary rejected saved-template equality and automatically rolled back. Sanaei moves its API rule to index zero. The fix preserves that API priority before pool rules, with a regression test. Fingerprint and exact readback checks were not weakened; original failure evidence remains.
- Fixed rollout passed 1 server, then 6, then all 38 serving servers. All 38 running route matrices were verified over the rollout window. One slow HTTP route-test path was independently verified through 38 read-only direct localhost gRPC probes, and its HTTP matrix subsequently passed at 14:39:13 UTC. A successful read does not prove the intermittent management delay is permanently fixed.
- The agent did not import example proxies or delete live endpoints. During final verification, an external live edit/import was observed: the former `Test` endpoint was replaced by 11 endpoints named `ads1` through `ads11`; one durable bulk import existed and routing revision reached 594. These edits were preserved. Fresh central observations included successful probes and intermittent timeouts. Earlier Test authentication rejection is historical, not the current endpoint state. A separate post-import verification is recorded without overwriting original rollout proofs.

## Evidence and operational limits

Evidence root: `/root/backups/dob-residential-pool-20261005`; final build/deployment evidence: its `v2` directory. See test-status/race/full/browser/pool logs, clean binary hashes, canary/six-state/fleet-state, final-proof/recheck/last-http-7d, direct-core-7d, completion-status and post-import proof/status files. Original failures are retained. The fleet-state initial PENDING marker is historical and must not be rewritten to simulate continuous success.

Management HTTP API verification can time out on loaded servers even while the loaded Xray matrix works. The worker retains truthful pending/failed verification states and retries reads; neither force-marking APPLIED nor blindly restarting a slow host is acceptable. No host reboot or forced Xray restart was done for the final direct-core diagnostics. Existing kernel KHO/CMA mitigation is inherited and unchanged; this feature does not guarantee absence of every future outage. Actual AdMob visual display, maximum bandwidth and live provider UDP capability remain outside the evidence.

## Safe rollback / continuation

Before reverting to a pre-pool worker, run this version with `DOB_RESIDENTIAL_POOL_PANELS=none` and verify reconciliation removes managed balancers, loopback rules and observatory references. An old binary alone cannot safely clean new pool references. Keep the backwards-compatible migration and encrypted endpoints; do not rewrite deployed migration 150, clear import ledgers or reset client lifecycle gates/jobs. Preserve unrelated worktrees and orchestrator ledgers. The exact scope and DNS/UDP isolation remain required.

## Latest post-import limit — 14:47 UTC

The separate revision-594 matrix completed at 32/38. Its six unconfirmed servers were checked directly: five timed out in localhost gRPC (including dial/RPC deadlines), and the sixth could not fetch the checksum artifact due to an HTTPS handshake timeout. These results supersede an interpretation that the remaining issue is only Sanaei HTTP overhead. No routing mismatch was established, but current complete fleet correctness is not proved. Preserve these unconfirmed states; the worker continues ordinary reconciliation. At 14:44 UTC the central snapshot reported 8 healthy and 3 timed-out endpoints, with 31 APPLIED and 7 FAILED verification rows; these are time-specific observations, not permanent labels. The new bulk and selection features are deployed; operational verification of the six remaining nodes is a separate outstanding limitation, not silently marked successful.
