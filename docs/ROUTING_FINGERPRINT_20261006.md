# Stable residential routing fingerprint — 2026-10-06

Problem reproduced: residential-only membership turnover changed outbound proof tags despite identical effective routing, causing unnecessary managed Xray reloads. Three live nodes showed repeated Xray PID changes while x-ui stayed running. CPU pressure coincided; causality and fleet-wide improvement are not yet quantified.

Stable mode fingerprints complete effective settings and inbound tags; direct-user rules, credentials, DNS, UDP, sniffing, pool and performance policy remain covered. Exact client membership receipts are persisted independently before native route verification, including creation/expiry. Strict proof, locks, policy fences and fresh membership readback remain mandatory.

Deployment selector `DOB_ROUTING_STABLE_PLAN_PANELS`: unset/all enables fleet scope; none restores legacy fingerprint; comma-separated panel IDs enable canaries. Initial scope cutover changes tags and can require one managed Xray reload. API and guardian binaries do not require replacement. Do not change operator profiles, limits, controls or account rules.

Validation: prior regression failed as expected; focused suite, full Go suite, focused race suite and installed Xray DNS/TCP/UDP/failure/recovery/performance rollback tests passed. Independent server-side OpenAI review resp_04a819bf143a330c006ac53d52865c87d1aa1a270439da31af: PASS, no blockers. Live deployment/turnover proof pending at source checkpoint.

Rollback: restore backed-up worker and manifest, remove this rollout drop-in, reload systemd and restart only worker. Legacy routing naturally reconverges with an expected reload. Keep all existing accounts, clients and operator settings.

Evidence: /root/backups/dob-hardening-20261006/fingerprint-* and .local/dev-orchestrator/routing-fingerprint-20261006-*.
