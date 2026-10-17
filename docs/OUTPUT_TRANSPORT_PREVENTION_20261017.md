# Transport/output recurrence prevention — 2026-10-17

The known transport lifecycle and fleet coverage regressions are now mandatory release blockers in the standard publisher. This follows product fix ab7989e and the verified public release 4356ba77a28ffe548a2b30f538144375b67101e2; no application binary, schema, credential or operator policy changed in this follow-up.

## Permanent controls

Existing runtime protections remain authoritative: pins are retired only after verified deletion; cleanup and transport admission share their fence; unfinished cleanup cannot admit a conversion; historical Restore cannot cross confirmed deletion into a recreated listener. Output describes verified native transport and retains profile ownership, expiry and freshness checks. Unknown write outcomes are not blindly retried.

The new publisher gate starts a disposable PostgreSQL as the unprivileged build user. It has no TCP listener, uses a private 0700 socket directory and never reads a supplied production database URL. The fixture is identified before testing and shut down/deleted afterward. Missing PostgreSQL tools or failed cleanup blocks publication. Root execution is rejected.

Every changed product candidate must execute and pass 10 named regressions with the race detector. They cover cleanup admission states, verified deletion and recreation, stale rollback, transport revision/scope rules, output locking and class separation, port/IPv6 filtering, stale/missing capacity observations and fleet coverage above 64 panels. The gate parses Go JSON test events: a removed/renamed test, missing pass, skipped test/subtest, failed package or malformed evidence blocks the public push. A green package exit alone is insufficient. Do not remove or bypass this requirement to make a release green; investigate the cause and rerun the exact candidate.

The gate runs in the publisher's existing isolated build service before its ordinary offline tests and install smoke test. Only its credential-free script is copied into the candidate evidence directory; the publisher's private 0700 directory stays private. No service credentials or live database are exposed to the builder. Metadata-only private commits still avoid duplicate product releases.

## Verification

- Seven evidence-verifier unit tests passed, including missing tests, skipped subtests and malformed/failing evidence.
- The real isolated database/race gate passed all 10 named tests with zero skips on the exact currently installed public 4356ba7 candidate.
- A real negative test removed the database environment: Go exited 0 with the fixture skipped, and the new gate rejected the result.
- The first negative integration exposed a private-directory permission error before test launch. It was corrected by staging the credential-free script in candidate evidence; the failed attempt is retained and is not labelled a passing test.
- Installed publisher/helper hashes match the reviewed source. Installation held the publisher lock and retained the previous publisher at /root/backups/dob-transport-gate-20261017.
- All six API/control/panel services on both hosts remained active, with zero automatic restarts.

## Live acceptance

From 01:23:44 to01:24:26 UTC (04:53:44–04:54:26 Tehran), three consecutive observations showed 93 healthy servers, 93 Direct links, 93 Residential links and 279 ALL links. All existing public shares matched; invalid links, missing healthy hosts, profile deficits and inactive/degraded/attention states were zero in this window. All 93 eligible lifecycle scopes were enrolled; cooldown, pause and quarantine counts were zero.

Earlier observations in this turn showed newly ready panels awaiting profile population and a brief initial-output delay. Existing automatic workers filled Direct profiles; two new panels then received one bounded standard current-policy reconciliation each to complete their profiles. No operation budget, creation cadence, rate limit, ownership check or health classification was overridden. This is a timestamped acceptance window, not a promise that future provider/host outages or provisioning transitions cannot occur.

XHTTP creation revision 8 and current Direct 1/Residential 1 targets remain saved. The previously explicit Restore keeps one valid TCP listener, so each profile output contains 92 XHTTP links and 1 TCP link. It must not be silently overwritten just to report zero TCP. This follow-up verifies live coverage and the release gate; prior full connection/native evidence is separately timestamped in OUTPUT_TRANSPORT_REPAIR_20261017.md.

Exact stages, observations and evidence hashes: docs/development-jobs/output-transport-prevention-20261017.json. OpenAI model API requests/tokens/cost: 0/0/0.
