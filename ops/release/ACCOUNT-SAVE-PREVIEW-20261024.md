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

Validation and deployment results will be appended after release gates complete.
OpenAI API: zero new requests/tokens/cost. Prior ledger: 3 calls, 6,809 tokens,
USD 0.05716284695 estimated; three-call cap retained.

Implementation references:
- https://pkg.go.dev/net/http#ResponseController
- https://nginx.org/en/docs/http/ngx_http_proxy_module.html#proxy_buffering
