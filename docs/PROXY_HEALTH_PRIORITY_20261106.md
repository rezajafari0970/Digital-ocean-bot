# Proxy health publication and primary identity release

Accepted at 2026-11-06T20:42:55.430921+00:00. Exact evidence and gate identities are recorded in [the JSON receipt](PROXY_HEALTH_PRIORITY_20261106.json).

## Problem and resulting behavior

An active provider/native browser held a proxy row SHARE lock. Nonblocking health writes skipped that row, so the base-proxy observation aged beyond the 360-second economy limit despite a healthy account route. The panel then recorded generic PROXY_ERROR/account network not ready.

Schema 207 replaces the browser's long proxy row lock with a transaction-scoped shared configuration pin. The migration guards material proxy updates/deletion using the corresponding exclusive advisory lock. Concurrent status, IP and geolocation health observations remain writable. Configuration changes during a pin return 409 with guidance to close the active browser. Secret lifetime protection remains, and unhealthy/revoked routes still stop dispatch.

Admission now records the exact failed prerequisite, including missing routes, unverified country policy, unavailable IPv4, unhealthy base proxy and stale identity/base observation. All seven provider diagnostic adapters retain these reasons.

The reconciler retries the committed primary identity even after a transient unhealthy/unknown mark. A verified primary keeps its IP, session and port. Unknown verification fails closed without a new allocation or unproven fallback. Existing configured fallback/failback rules remain. Saved TT/US/TV priorities and both legacy AUTO account policies were not changed.

## Verified release boundaries

- Tested source: cae452cec2d6e5a2c711365b1724d82cd118c73b; PR 48 merge: 86a8f3fa819556543ba4dedee8d754fb1a5ec89d.
- Canonical source repository: rezajafari0970/Digital-ocean-bot, branch checkpoint/final-e2e-20260929.
- Public main/stable and both installed runtimes: 73fa40ce8d4d6ec6f7eb947d0ac0d8fb2ab70aed.
- Full source workflow 42286251434: 380 required roots passed, zero skips; full checks and restore drill through migration 207 passed.
- Focused workflow 42286251553: health/geo writes during pins, forbidden config edits/deletes, unhealthy zero-dispatch, UUID aliasing, 12-cycle primary continuity, native CONNECT/TLS and all-provider diagnostics passed.
- Independent public workflow 42286574249: test, Ubuntu 24 fresh install and repeat-install identity preservation passed.
- Promotion workflow 42287956673: verified exact source/public gates and atomically promoted main/stable.
- An earlier source run 42285855811 failed an existing scroll delta fixture. Its receipt is retained; the final source passed that step without frontend changes.
- Export gitleaks and configured live-secret scans passed. No secret values were logged.

## Installed acceptance

Both installations completed the native bounded updater with 1 GiB memory/one CPU limits, backups and schema 207 migration. All 21 preservation groups per instance passed, including credentials, account/country policies, proxy configuration, login identity and existing profile ownership. Six running binaries matched manifests, ten readiness checks passed, and HTML plus 32 assets per instance passed, including normal public TLS.

A temporary native session stayed open 390 seconds. 14 health samples remained within the freshness limit (maximum 296.1 seconds), with 3 distinct monitor success timestamps while the pin remained held. The explicit primary country was retained in 14 samples, and all three committed account IPv4/sticky identities remained unchanged. Real assigned-proxy provider health checks passed for one Vultr and two Linode accounts both before and after the hold. The three active accounts had no unresolved provider error at final observation; errors were not manually cleared.

The first live attempt stopped when the temporary native route lost verification after one health sample. The exact first closure cause was not captured; its failed receipt is retained in live-attempt-1. A second diagnostic attempt captured native state and fail-closed evidence; only the successful completed hold supplies the duration results above. This transient failure limits availability claims.

The exported client exercised public WSS, original HTTPS, expected pinned exit and UDP DNS redirected through tunnel DoH. Revoking the session stopped subsequent client traffic without direct fallback. The temporary operator/session/client were removed; acceptance performed no cloud resource mutations.

## Limits and continuation

This bounded observation is not a 99% long-term availability or permanent-IP guarantee. DataImpulse residential nodes can go offline and replace an address even within sticky-session lifetime. The route must be reverified before use; losing verification blocks access. Explicit priority retention is measured here only for the account with an explicit primary country. Legacy AUTO accounts retain their existing policies.

Diagnostics were tested for all seven provider adapters; live coverage was limited to the three saved active accounts above. No provider login, CAPTCHA bypass or physical Android interaction was tested in this release. The previous native-Chrome user flow remains available; remote-renderer tests alone do not prove physical-phone parity.

Do not start the quarantined original primary cluster/duplicate workers or replay unknown Kamatera creates. No rollout intent remains pending. Development/evidence root: /root/dob-proxy-health-20261106. Operational scripts, failed/successful CI receipts, before/after snapshots, live-proxy-acceptance.json and final-observation.json are retained there. This final documentation checkpoint does not change installed product code.
