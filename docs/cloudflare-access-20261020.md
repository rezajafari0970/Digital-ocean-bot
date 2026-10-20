# Cloudflare DNS permission incident — 2026-10-20

## Evidence

The 56.8-second video shows a generic permission error (6 seconds), hundreds of pending record intents presented as managed records (14–36 seconds), an unusable probe button (38 seconds), and the WS+TLS profile with zero published configs (46–56 seconds).

Read-only live checks with the existing account credential:
- Zone read: HTTP 200.
- SSL setting read: HTTP 200, strict.
- Origin certificate list: HTTP 200.
- DNS record list: HTTP 403, provider code 10000.
No credential values or provider response bodies were logged. DNS write and certificate issuance are not proved by successful zone/certificate reads. The user must grant the appropriate token permissions for the selected zone.

## Correction

- Fixed operation labels distinguish DNS read/write, certificate and SSL permission failures without persisting provider bodies.
- Migration 000190_cloudflare_dns_access introduces independent per-zone read checks and five-minute retry windows.
- Scheduling requires fresh successful DNS access. Failed zones cannot fill new intent queues or monopolize healthy siblings.
- Expired PREPARED intents without provider or activation receipts can retire locally. SENT outcomes remain journaled and require fresh provider reads.
- New unsent certificate work waits for a usable domain. Valid installed certificates, node repair and ambiguous issuance remain recoverable.
- Account save and domain refresh invalidate the access cache, allowing corrected credentials to recover.
- Status separates tracked requests, pending requests, confirmed DNS records and probe-ready records.
- Probe availability uses the same complete publication predicate as the actual probe/output endpoint.
- Mobile records wrap safely, paginate 25 per page and disable unavailable probes.

## Validation

Race-enabled Cloudflare tests pass in four packages. Four new mandatory regressions cover operation redaction, independent access/recovery, unsent versus uncertain retirement, and truthful counters. Existing output revocation tests now check probe parity too. Certificate tests cover recovery and retained valid certificates while DNS access is denied. All 14 Chromium mobile cases and seven release-gate verifier tests pass.

## Rollout

Pending mandatory publication and staged installation. Credentials, selected domains, existing profile policies and client identities must remain preserved. DNS and real certificate completion remain blocked until the external token permissions are corrected.
