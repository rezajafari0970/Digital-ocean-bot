## Output persistence incident and forward fix — 2026-10-15
New native XHTTP listeners were being created and their links generated, but the old output_config_snapshots primary key indexed the complete URI. Production PostgreSQL reported index rows4136–4184bytes above2704; persistence errors were silently discarded. The earlier canary/URI checks did not cover the long URI through this materialized-output table.
Migration179 replaces the unbounded URI index with a bounded digest, preserving full URI data and existing timestamps. Conflict updates require exact URI equality and count checks abort any collision. Failed snapshot transactions preserve prior data and emit credential-free stage/SQLSTATE diagnostics.
Regression: actual4096-byte exported paths fail on original source(0/3 stored); fixed full URI storage/shared output/refresh/dedup/stale removal pass. Upgrade preserves TCP and first_seen; injected DB failure returns an error and retains prior snapshot. Focused race plus full isolated PostgreSQL/browser suite pass. Publication and live output acceptance are still pending at this source checkpoint. No listener/path/credential/default conversion or unrelated IP lifecycle work.

# XHTTP listener profiles — 2026-10-15

## Contract
Creation defaults affect only new physical listeners. Existing TCP listeners are not converted.
- XHTTP Reality, stream-up; apart.com Host; existing SNI selection.
- ASCII path256–4096bytes, vocabulary shuffled without duplicate segments, random128-bit suffix, durable pin per panel/managed listener.4096 is a tested application limit, not universal maximum.
- Server-only32768header budget avoids HTTP431 caused by long URI/Referer. Client Extra remains the exact requested six-field object by JSON value/type/order-of-arrays.
- Client Flow is empty; WebView is disabled. API rejects incompatible combinations and omitted network.
- Exact requested FinalMask stored without double wrapper, disabled; no fm URI parameter while unsupported. Installed26.9.9 rejects numeric brutalUp and UDP exp.
- UI/API revision-CAS, stable path persistence, client creation/execution flow validation, fresh URI exports, capacity and firewall parsing are integrated.
- New migration178 is additive; filename-keyed runner does not suppress the separate, unmerged177 IP work.

## Evidence
All36reviewed product/test hashes identical to source-review PASS. Full-final project/isolated PostgreSQL/browser suite passed. Final affected race suite passed, including previously skipped transportprofile tests. Browser-r3, vet and diff passed.
Native X-UI canary on existing server:4096-byte path, Host and client Extra readback verified; repeated create found same inbound; one guarded restart retained payload;16/16allowed phone HTTP requests passed before/after restart. Scoped canary removed.
Previous Cloudflare trace failed because it is outside Residential's allowlist. Allowed infrastructure probes and ad assets succeed; routing restrictions retained.
Phone operator was not identifiable in this continuation. Static ad assets do not prove SDK ad display or long-term stability.
Runtime publication, rollout and activation evidence will be appended after completion. No new cloud servers/IP allocation, unrelated IP lifecycle work or credential rotation.

## Rollback
Switch creation default back to TCP with current policy revision to stop future XHTTP creation. Existing pins and clients retain their network/path. Do not downgrade the binary to a TCP-only version while XHTTP listeners are serving; use a reviewed forward fix or bounded retirement first. Preserve additive schema.

## API use
7completed model responses:61103input+13941output=75044tokens. No new inference this continuation; prior5council receipts read by GET. Billed amount unavailable.

## Deployed acceptance — 2026-10-15T16:23:43.399698+00:00
- Product source: d671e194a29223f15aa8599fc4e88335eb060486.
- Public/runtime:89292e72b9feb2addebaf1bd4dd39e12b09d216c; both main/stable Actions passed.
- Both management hosts: schema178, three active matching binaries, five successful readiness checks, identity/environment preserved, actual transport API and asset verified.
- Creation default revision2 is XHTTP stream-up/apart.com/4096bytes on both hosts. Existing TCP listeners and pins remain TCP. No ordinary fleet XHTTP listener observed yet after activation.
- Final dashboard39tracked:29active,5provider-unavailable inactive,5pending deletion,0degraded. Provider recovery is outside this change.
- Exact FinalMask and WebView remain unsupported for the tested combination. Empty Flow is intentional.
- Temporary native38445 and independent38444 tests cleaned up; user's separate38443 manual test was not touched.
- No new external model inference; total task usage75,044tokens in7responses.
