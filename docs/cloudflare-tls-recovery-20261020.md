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

Publication and sequential installation are pending. Real TLS publication and the manual refresh action must be verified after deployment. Capacity above the selected zone's provider quota remains an external limit; do not claim all requested generations fit without additional selected domain capacity.

Official contracts: https://developers.cloudflare.com/api/resources/origin_ca_certificates/methods/list/ and https://developers.cloudflare.com/api/resources/dns/subresources/usage/subresources/zone/methods/get/.
