# Cloudflare certificate recovery and DNS capacity — 2026-10-20

## Incident evidence

The 85.9-second video shows 0/73 ready WS+TLS servers, generic retry messages, confirmed DNS rising from 93 to 135 with no published configs, and `cloudflare_busy` on manual refresh. The prior DNS credential restriction has been corrected externally.

Read-only provider checks confirmed three wire-contract gaps: certificate expiry uses `2006-01-02 15:04:05 -0700 MST`, list results omit CSR, and the default list is paginated at 20 records. All 74 provider certificates matched current journaled CSR public keys; 73 belonged to live nodes, and all 74 signatures verified against the pinned Cloudflare origin root. No issuance was performed by these diagnostics.

The selected zone also reports `record_usage=201`, `record_quota=200`. Foreign DNS records remain untouched; additional unselected domains must not be enabled automatically.

## Correction

- Treat API expiry text as informational; certificate lifetime, SANs, chain and public-key binding are validated from signed X.509 data.
- Read all certificate pages with a 50-record page size and bounded traversal. Reject repeated or incomplete pages rather than interpreting a partial list as proof of absence.
- Recover uncertain issuance by verified certificate public key and persist its provider receipt, even when the API omits CSR. Never resubmit an uncertain CSR.
- Manual account actions wait at most five seconds for the account lock; only acquisition retries, and the action executes at most once. Background workers retain nonblocking locks.
- Preserve actionable Cloudflare error labels when wrapped in profile error codes, and show refresh progress.
- Migration 000191 stores per-zone official DNS quota/usage observations. Check capacity each minute. Reserve capacity for pending and newly confirmed records; pause only new creation when full. Existing record observation, certificate work and expiry cleanup remain independent.
- An unavailable quota endpoint is reported explicitly without preventing ownership checks and cleanup. No domain selection, credential, profile schedule or residential routing policy is changed.

## Validation and rollout

Targeted race tests pass in all four packages. All 16 Chromium mobile cases and seven gate-verifier tests pass. Five new mandatory tests cover real certificate payloads/pagination, incomplete-list rejection, bounded lock acquisition without mutation replay, quota exhaustion/recovery with independent cleanup, and partial-batch interval preservation. Existing lost-response tests now recover CSR-less certificates while rejecting forged metadata and checking the persisted provider receipt.

Source `ece3aa81a999199575cecd7432802062bba0e760` was published as `2a9d64bff8df46e85a28354caa0d4fc1444b6af6`. The exact candidate passed all 71 required release tests with zero skips, the full Go suite/vet, installer tests, six output-browser cases, and isolated installation/identity checks. Both production controllers now run that public revision and migration 000191: all five readiness checks and all three executable hashes match, with zero service restarts.

Primary preservation verification covered 3,006 existing profile identities and 96 Trojan identities. No unexpected credential or identity changes occurred. Seventy initially unprovisioned Cloudflare listeners acquired their first native IDs while retaining their existing credentials; nine expired listener retirements were confirmed. Initial activation is proved by the matching native mutation receipt/event, exact applied specification/revision, installed certificate and unchanged credential hash. Network readiness is measured independently; DNS capacity must not be mistaken for an identity change. Both controllers retain their master key, environment, policies, domain selection and selective Google Ads routing.

Manual account refresh succeeded in 0.618 seconds. Authenticated status, IP-pool routes and both served JavaScript asset hashes passed. Certificate recovery installed all 70 live-node certificates during the first recovery cycle. At 18:12:39 Tehran, normal server expiry had reduced the eligible fleet to 67: all 67 certificates remained installed, 61 Cloudflare listeners were verified, and 59 servers had 118 currently publishable class outputs out of a target of 134. The other three profiles each had 67/67 ready servers and 134/134 outputs. Local output API latency was 0.047 seconds; this is controller latency, not an Iranian mobile-network speed measurement.

Remaining external constraint: the one selected zone has a provider quota of 200 records. It reached 200/200; automatic expiry cleanup subsequently reduced observed usage to 194 and the existing creation queue resumed. Six listeners still reported DNS pending, with two additional output-readiness items at the snapshot. Across the journal, pending records fell from 150 to 128 while confirmed deletions rose from 391 to 415, with no expired unsent records left. The chosen two-record/30-minute schedule with 45–60-minute lifetimes can exceed a single 200-record zone across this fleet. Additional user-selected domain capacity or a provider quota increase is needed to sustain the full schedule; no unselected domain, foreign DNS record, lifetime or record-count setting was changed. Real Iranian-operator measurements remain unavailable.

An earlier source candidate (`c213f02`) was stopped before public push or installation when final review found that a quota-limited partial batch could fail to consume its interval. The published source includes that correction and its mandatory regression test. OpenAI API usage remained zero.

Evidence is retained at `/var/lib/datacenter-publisher/release-7vnrjir1` and `/root/backups/dob-cloudflare-tls-20261020` on the primary controller, with the secondary installation/verification in the same backup path on `limited-lime.ptr.network`.

Official contracts: https://developers.cloudflare.com/api/resources/origin_ca_certificates/methods/list/ and https://developers.cloudflare.com/api/resources/dns/subresources/usage/subresources/zone/methods/get/.
