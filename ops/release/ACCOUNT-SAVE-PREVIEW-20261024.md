# Account Save and provider preview correction — 2026-10-24

Baseline private checkpoint: d72a2a925e6c3eebe2351f8a239ab26b40fe4cb1.
Baseline public and installed release: ae5f2e8886fb9dd2dcb5e161aacb44847cfe9445.

The entire 144.8-second recording was reviewed at one-second intervals; audio is silent.
At 35 seconds it shows raw 502 gateway HTML after a long multi-region preview.
Save was deliberately hidden whenever preview was invalidated, without a useful
progress or save-readiness indicator. Production diagnostics at 08:26:55 UTC record
a catalog transport failure after 39,643 ms, beyond the API server's 30-second write
timeout. AWS discovery and regional catalogue reads each allow up to 75 seconds.
This deadline mismatch explains loss of useful provider errors at the web boundary.
The precise external proxy failure is not established by the recording alone.

The account form now keeps Save visible in a fixed action area with an explicit
disabled reason. Only its fields scroll. Validation and required network consent
still gate saving. Preview requests are cancellable on changes or closing the form;
old results cannot re-enable Save. Failed validation preserves entries. Raw gateway
HTML is replaced with a concise error.

The authenticated read-only preview endpoint accepts application/x-ndjson for
request-scoped stage/elapsed progress and one terminal envelope preserving the
original HTTP error status. Heartbeats are flushed every five seconds, buffering
is disabled for supporting reverse proxies, and only this response receives a
175-second write deadline. Work is bounded to 165 seconds, the browser to 180.
Legacy JSON requests remain supported with a 25-second work context, below the
normal 30-second write deadline. There is no persistent preview job, background
credential store, automatic retry, or provider mutation. Request and response sizes
are bounded; provider work is cancelled when the connection is lost.

Additional AWS region networks are validated with at most three concurrent reads.
Duplicate and disabled selections are rejected before fan-out, results merge in
selection order, and incomplete results never publish partial credentials.

Verification includes server write-deadline survival, cancellation, timeout, error
status preservation, malformed/truncated responses, request size limits, race
detection, and independent subnet ownership under three-way concurrency. The real
mobile browser covers 320x480, 390x420, 844x390 and 390x844, Save hit-testing and real
pointer submission for AWS and OVH, popup allowed/blocked, progress, gateway errors,
truncation, stale selections, scheduling draft preservation and required consent.

Validation and deployment completed; exact results are below and in the companion evidence JSON.
OpenAI API: zero new requests/tokens/cost. Prior ledger: 3 calls, 6,809 tokens,
USD 0.05716284695 estimated; three-call cap retained.

Implementation references:
- https://pkg.go.dev/net/http#ResponseController
- https://nginx.org/en/docs/http/ngx_http_proxy_module.html#proxy_buffering


## Completed validation and release

- Full Go test suite and go vet passed. Targeted admin API and AWS race tests passed.
- Installer tests: 12 bootstrap and 11 update tests passed. JavaScript syntax and decoder tests passed.
- Mobile browser passed with popup allowed (12.20 s) and blocked (12.16 s), including real pointer Save submissions against synthetic AWS/OVH fixtures at the tested viewport sizes.
- The new browser regression failed on the old static assets at the initial Save visibility assertion, demonstrating coverage of the reported bug.
- Canonical ops/release transport/fleet gate passed all 156 required checks with zero skips.
- Secret scanning passed against both installations and the public export; gitleaks passed.
- Exact public candidate 6336c9e47f2907adc029024ea05fa433346b7e62 passed CI 40079957730, including independent initial and repeat installation with preserved identity.
- Promotion 40080123759 atomically advanced main and stable from ae5f2e8886fb9dd2dcb5e161aacb44847cfe9445 to the verified candidate with expected-head leases.
- Both default and production installations completed their guarded update with exit 0. Backups and configuration baselines preceded each update.
- Each installation passed five readiness checks, executable/manifest identity, and fourteen preservation checks. Account settings, credentials, encryption key, policies and Cloudflare settings were preserved.
- Public HTTPS passed normal certificate verification, exact release identity and byte-for-byte comparison of aws-catalog.js, preview-stream.js, app.js and app.css. Both integrity timers remain enabled/active; no pending deployment marker remains.

### Validation corrections and limits

The first production attempt stopped at the unchanged disk-reserve guard before any service switch. Its log and exit status were retained. Removing unused, reproducible Chrome 155.0.8059.39 download caches recovered 700,465,152 bytes (no active browser or service reference was present). An additional 252,379,136 bytes net were recovered from the unused prior Go test cache and reproducible APT metadata caches while the new backup was running. The exact previously verified staged candidate was resumed with the normal production locks, integrity, reserve, backup, admission, identity and readiness checks intact; no database, runtime rollback or backup was deleted for this recovery.

Initial browser runs exposed an outdated assertion expecting the normal Validate label during its new busy state; sequencing was corrected and both complete scenarios passed. An initial invocation of the obsolete tools/public-release gate lacked the native geosite dependency and also encountered a three-second queue timeout. The documented ops/release gate was then used with the pinned, hash-verified dependency and passed all 156 checks. Product behavior was not changed to suppress these failures.

This release fixes Save visibility and preview progress/deadline handling. It does not claim that external AWS/proxy connectivity can never fail. Actual AWS creation/deletion was not used for testing; browser provider submissions used isolated synthetic fixtures. No credentials from the screen recording were replayed or published. Reopen the account form after refreshing the page to load the new static assets. Save remains disabled until validation and required network consent are complete, with the reason visible in the fixed action area.

Final read-only saved AWS account counts: {"default": 0, "production": 0}.

default identity preservation: {"profile_ownership": {"common": 0, "confirmed_expired_retirements": 0, "confirmed_initial_activations": 0, "confirmed_preupdate_initial_activations": 0, "new_since_backup": 0, "retired_since_backup": 0, "unchanged": 0, "unexpected_changes": 0}, "trojan_ownership": {"common": 0, "confirmed_expired_retirements": 0, "confirmed_initial_activations": 0, "confirmed_preupdate_initial_activations": 0, "new_since_backup": 0, "retired_since_backup": 0, "unchanged": 0, "unexpected_changes": 0}}.

production identity preservation: {"profile_ownership": {"common": 5080, "confirmed_expired_retirements": 4, "confirmed_initial_activations": 0, "confirmed_preupdate_initial_activations": 0, "new_since_backup": 4, "retired_since_backup": 0, "unchanged": 5076, "unexpected_changes": 0}, "trojan_ownership": {"common": 77, "confirmed_expired_retirements": 0, "confirmed_initial_activations": 0, "confirmed_preupdate_initial_activations": 0, "new_since_backup": 0, "retired_since_backup": 0, "unchanged": 77, "unexpected_changes": 0}}.

Completed UTC: 2026-10-24T09:19:25.132371+00:00. New OpenAI API consumption remains 0 requests, 0 tokens, USD 0; prior cumulative ledger is unchanged.
