# Speed tuning publication and recovery diagnostics — 2026-10-11

## Problem and scope

The 3-path/90% trial was acknowledged by serving panels, but publication included old surviving assignments whose panels were already unreachable. A panel API failure requested conservative fleet restoration at 09:15:47 Tehran. The persisted reason omitted the panel/operation. The admin page labelled the parent KEPT profile PUBLISHED while the active tuning remained RESTORING.

Thirty-one legacy panels belong to disabled, deletion-requested Vultr accounts Sara vul and Sara vul 2 with TOKEN_INVALID. These remain real recovery obligations until matching fresh runtime proof or authenticated provider-confirmed deletion. This change does not clear their records, relax freshness, settle RESTORING, republish 3/90, or claim ad latency improvement.

## Changes

- Atomic tune_publish preflight requires matching fresh runtime receipts for every surviving owned target before any fleet candidate assignment. Existing owner/configuration checks distinguish candidate canaries from unchanged baseline targets. Missing targets and marker drift remain conflicts. Rejection leaves canary, deadline, profile and assignments unchanged.
- The first matching failure that requests restoration records only panel UUID, generation, database time and a closed-enum operation stage. Later failures cannot replace that trigger. An operation stage does not assert whether a timed-out mutation occurred.
- Admin status exposes uncapped survivor/fresh/pending/non-serving/invalid-credential/missing-record totals. The target detail table remains limited to 200 rows.
- Admin UI gives active TESTING/PUBLISHING/RESTORING precedence over the parent publication label and displays recovery blockers and first-trigger metadata. Historical failures without metadata are explicitly identified.

No domain, DNS, protocol, proxy credential, lifecycle expiry, residential egress, selection profile or retirement change is made. No schema migration is required. Android source is out of scope.

## Verification

Focused TestTuning PostgreSQL tests PASS, including atomic rejection for an unreachable RETIRING invalid-account target, both freshness clocks, router/application generation mismatch, first-trigger preservation/redaction and read-only incomplete-recovery counts. Existing coverage/lost-target/rollback/idempotency tests also PASS.

Bounded relevant race tests across residentialperf, residentialsync and adminapi PASS. Full general go test ./... PASS without production DB access; opt-in database/native suites are not claimed by that gate. Affected database tests used the isolated bulk_test database. Real Chromium performance workflow plus mobile recovery label/counts/HTML escaping PASS. The first browser run failed due to the added fixture's top-level await; the fixture was corrected and the browser test passed.

Exact-source API review completed with PASS and no blockers. Reviewed digest: c6f62e92d6837ee6dd80e8c9c20c8541bdbe51e85b4973699f4dda74f2749111.
Response ID: resp_0b93940d8450a0af006acb2b493bc487d1b6756141c52a602f.
Two separate OpenAI API requests: 22,152 input + 3,539 output = 25,691 total tokens. The first design response was incomplete and was not acceptance. Billing cost is not available from these usage receipts.

## Release and remaining work

Prepared for a clean detached build, protected publication and service/hash/readiness checks. Consult /root/backups/dob-speed-recovery-20261011/deployment.json and status.json for the actual deployment outcome; source preparation alone is not a runtime receipt.

The existing 13/80 profile remains requested. Recovery still requires fresh panel proof or genuine provider deletion. Repair the two account API credentials through the normal account editor to restore provider visibility; do not send secrets to chat, force DELETED/RESTORED or disable fail-closed behavior. After recovery completes, repeat only the necessary guarded single-panel speed trial before publication.

## Verified deployment receipt

Runtime 93b343757e5b36265c8a01f31e089ded07e179df was published at 2026-10-11 09:55 Tehran from a clean detached build. API and both workers active, exact runtime hashes matched, all five readiness checks passed, selectors remained all. No automatic service restart observed in the release receipt.

At 09:56 Tehran exactly one existing panel was checked through its API: seven asset domains routed to residential and fundingchoicesmessages.google.com remained blocked according to the existing seven-root policy. Eight native route assertions passed. Native observatory responded; DNS retained TCP 1.1.1.1/8.8.8.8, IPv4 and caching. This is routing evidence, not an ad-ready or download-speed gain.

The production diagnostic query returned version8 RESTORING on13/80, 76 surviving obligations,27 freshly verified,49 pending,34 non-serving,31 invalid-credential blocked and0 missing recovery records. These are timestamped observations, not permanent counts. No old recovery records were removed or acknowledged without evidence.

A concrete next routing proposal, NOT APPLIED, is to add domain:fundingchoicesmessages.google.com to the residential category extension (that host and its descendants/all URL paths), leaving broad google.com and the other removed roots unchanged. The previous explicit seven-root restriction requires user clarification before this additional domain is deployed. Its blockage is proven; its causal contribution to the recorded18-second app delay is unproven.
