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

Source `ff09e25f192fb00ad6ab84aeec2d978707635627` was published as `b5f6143e9fe620ae467e4e32040e8fd5f0d2f59e` after 66 mandatory race regressions (zero skipped), full Go tests/vet, 14 installer tests, six output tests, and exact-revision LXC installation. Both production panels were installed sequentially and passed all five readiness checks on schema 000190. Running executable hashes match the release, with zero service restarts at verification.

Master keys, environments, Cloudflare credentials/settings, profile/output/protection policies and selective Google Ads routing are preserved. Of 2,990 pre-existing profile identities, three only underwent confirmed expired-server retirement; there were zero unexpected changes. All 96 Trojan identities remain unchanged. Three existing profiles are ready on 71 eligible servers, publishing 426 outputs; the new Cloudflare profile remains blocked externally.

Live status now reports zero confirmed DNS records and zero probe-ready records, instead of presenting pending requests as created records. At final observation, 304 unsent expired/obsolete intents had retired and 144 nonexpired intents remained pending. No new intents appeared during a 52-second observation; DNS check time remained unchanged, with the next attempt scheduled five minutes later. Served JavaScript matches the installed source.

**Remaining external blocker:** the connected token still receives DNS read HTTP 403 (provider code 10000). Grant DNS Edit for the selected zone, then use **Refresh domains and access**. Successful zone, SSL-setting and certificate reads do not prove DNS write or certificate issuance permissions. Origin certificate issuance requires SSL and Certificates Edit; actual DNS creation, issuance, end-to-end TLS and published Cloudflare configs remain unverified until the external permission is corrected. No credential values were logged and no OpenAI API calls were made.
