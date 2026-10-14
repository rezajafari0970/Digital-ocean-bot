# Direct MCI investigation — 2026-10-14

Status: **direct Irancell sidecar path verified; MCI solution and v2rayNG application acceptance not established; experimental project changes only**.

## Scope and constraints

- No changes to the v2rayNG application or its embedded core.
- No additional traffic relay. All probes connect directly to the existing target; the synthetic origin is loopback on that same target.
- The phone sidecar uses the verified official Android/arm64 Xray 26.5.9 binary. This is a protocol experiment, not proof of v2rayNG import, Android TUN, app routing, DNS or reconnect behavior.
- Production listeners, credentials, advertised SNI, routing, kernel sysctls and the production JSON remain unchanged.
- Tests use the existing lifecycle/host-key/account guards. An expired or nearly expired target must not be reused by bypassing those guards.
- Preserve the canonical project checkout and the existing advanced-reality branch. This work is isolated in `feat/mci-evidence-20261014`, based on `b50cede096a76f1cbdc956f53f3d57ac368b801b`.

## Experimental findings

A temporary, certificate-verified synthetic endpoint consumes a bounded upload and returns the unique request nonce, the server-computed SHA-256 of the upload, and a known complete response body. No third-party website availability or DNS is needed to establish payload success.

The first control batch was **invalid setup**: the core's default private-address guard correctly blocked the loopback fixture. It was excluded. The fixture was corrected with an exact exception for its own `127.0.0.1:38880` origin in its isolated configuration. Production private-address protection was not weakened.

| Batch | Off-carrier control | MCI observation |
|---|---:|---|
| Ordinary VLESS/TLS 1.2 and 1.3 plus independent HTTPS | 9/9 | 2/9: the tiny VLESS exchanges passed; larger payloads failed |
| Client MSS default / 536 / 900, direction separated | 9/9 | 1/9: default tiny passed; none of the large transfers passed |
| Three repeated directional runs with before/after tiny controls | 12/12 | Tiny controls 5/6; 64 KiB download 0/3; 16 KiB upload 0/3 |
| Application pacing: 256 B writes / 10 ms for download; 2 KiB/s upload | 12/12 | Tiny controls 6/6; download 0/3; upload 0/3, with an 18-second deadline |
| Same-port plaintext HTTP versus TLS 1.3, two runs per size/direction | 12/12 | Small HTTP 2/2 and small TLS 2/2; all eight large transfers failed |

Payload sizes above exclude HTTP headers and the 98-byte nonce/hash prefix. The initial directional download received 8,131 of 65,634 expected body bytes. Repeated default downloads received 3,495 or 8,131 bytes. Paced downloads received 3,523 bytes consistently in three trials. Partial HTTP 200 responses are **failures**, not working configurations.

Initial controls used Linux Xray 26.3.27; the pacing and same-port batches use Linux 26.5.9, matching the phone release. OS, architecture, curl and OpenSSL versions still differ. These differences must remain visible in interpretation.

Server capture verified phone SYN advertisements of MSS 1400 (default), 536 and 900. Lower MSS did not recover transfer. The server trace contains retransmitted sequence ranges and a lack of normal ACK advancement after the initial exchange. Offload/GRO/GSO can produce apparent packet sizes larger than the advertised MSS: analyze sequence space, not host-capture lengths alone. Server-side capture alone cannot locate the loss or establish what Android received.

**Supported conclusion from the MCI batches:** the target is not wholly unreachable, and neither all TLS nor all authenticated proxy traffic is blocked. There is a reproducible larger-transfer problem on the tested path. These observations do not prove which middlebox, network segment, endpoint, MTU behavior or DPI mechanism caused it, and do not establish successful REALITY or usable browsing.

## Durable project changes

1. `Inspect` rejects the known unsupported REALITY inbound / TCP FinalMask composition, including an unchanged unsafe baseline. Client outbound fragmentation is outside this server-side prohibition. Native configuration validation is still mandatory.
2. `AssessConnectivity` and the lab CLI's `evidence` action assess a bounded operator report. They reject ambiguous or sensitive JSON, unknown fields, stale/future timestamps, missing exact core/config/target hashes, duplicate nonces within a report, invalid payload-success claims and missing route/control context.
3. A handshake, header, tiny response or partial body cannot become a bulk success. At least three complete bidirectional bulk samples are required for `BIDIRECTIONAL_SAMPLE`.
4. **Every assessment retains `can_apply:false`.** Reported booleans and hashes are not authenticated device attestations, and no cross-report replay database is implied. Raw TLS, sidecar and v2rayNG paths have distinct remaining acceptance blockers.

This is an observation/validation feature, not a rollout actuator or an automatic DPI bypass. It is not deployed fleet-wide and must not be advertised as anti-filter immunity.

## Next acceptance gates

- The Irancell paired run below is complete. Return to MCI for an A-B-A confirmation while holding destination, official client binary, payload, transport and timeout fixed and honoring target expiry.
- A bounded packet trace and client/origin nonce/hash correlation; no payload or credential logging.
- A successful profile must pass full upload/download, DNS, official v2rayNG import/TUN, sustained traffic, screen-off, network switch and reconnect checks before promotion.
- Compare connect latency, time to first byte, useful throughput, timeout rate and reconnect time against the unchanged control. A setting is not better just because it fragments more.
- Kernel-wide BBR, MTU, offload, sysctl and queue changes require evidence of a specific transport bottleneck, one change at a time, with rollback. They do not by themselves remove IP or protocol blocking.
- Server-side half-close handling must be redesigned and independently tested before reconsidering unsupported REALITY/mask wrappers. Replacing half-close with full Close can truncate the other direction.
- Preserve known working profiles and bounded per-carrier acceptance records. Any future controller needs current evidence, expiry, concurrency/resource limits, canary isolation, rollback and explicit uncertainty; no permanent unfilterability claim.

## Astra API accounting

Two explicit Responses API requests used `gpt-6-astra`, `store:false`, standard tier, with sanitized summaries only.

| Request | Effort | Input | Output | Total | Estimated USD | Result |
|---|---|---:|---:|---:|---:|---|
| Initial review | max | 1,252 | 16,000 | 17,252 | 0.8156425 | Incomplete: all output budget consumed by reasoning, no final answer |
| Bounded follow-up | high | 495 | 3,434 | 3,929 | 0.1766500 | Completed and used for directional/MSS/pacing test design |
| **This stage total** | | **1,747** | **19,434** | **21,181** | **0.9922925** | |

Reasoning tokens are already included in output tokens. The first request reported 1,249 cache-write tokens and no cached-input tokens. Price calculation uses the published standard input/output/cache-write rates; it is an estimate, not an invoice. This table excludes ChatGPT usage and previous stages/account usage. The ChatGPT model selection was not changed programmatically.

## Evidence and verification

Private raw receipts, command results and packet captures are kept in the owner-managed diagnostic directories; no keys, UUIDs, production configuration or live IPs are committed here. The isolated fixture is short-lived and must be explicitly cleaned. See the final verification update for cleanup, source tests and checkpoint status.


## Final verification update

- Same-port plaintext HTTP bulk transfers also fail. A TLS-only/SNI-only or VLESS-only diagnosis is not supported. Plaintext downloads stopped after 1,327 and 15,207 body bytes; TLS downloads did not deliver a complete body. Neither plaintext transport nor certificate bypass is proposed for production.
- The local nftables ruleset was empty; iptables INPUT/FORWARD/OUTPUT policies were ACCEPT with no rules; the observed egress qdisc had zero drops and zero backlog. These observations do not exclude provider filtering, return-path loss, cellular middleboxes, endpoint behavior or other causes.
- The five temporary listeners were closed and four fixture units stopped. Test TLS private key and isolated VLESS credentials/configurations were removed. Main port 443 remained listening; production JSON SHA-256 matched the pre-test value.
- Server packet metadata and origin logs were copied to the manager through the existing guarded/pinned transport and archive SHA-256 verified. Header metadata does not prove phone-side packet reception.
- Focused final acceptance: **77** contract/native/race cases and **3** admin cases passed; zero failed or skipped. Native test server 26.10.10 and test client 26.5.9 were recorded by the existing acceptance harness.
- Broad repository gate before the final case-alias guard hardening: **1,779 passed, 41 skipped, zero failed**, exit 0; full go vet exit 0. Skips are pre-existing opt-in browser/live/core/namespace/database suites, not claimed as passed. After that small guard hardening, focused native/race/admin tests, package vet, CLI build and JS syntax checks passed again. No application UI was changed.
- CLI end-to-end synthetic check: a complete report returns a non-applying sample; a false download-success claim is rejected. This is not network acceptance.
- No new API request was made after the two Astra receipts above.
- **Updated external requirement:** the Irancell paired run below is complete; a return to MCI is needed for A-B-A confirmation. Switching the active SIM/network cannot be assumed or performed through the available nonroot Termux shell. The MCI solution, official v2rayNG acceptance, sustained-throughput improvement and fleet activation remain unverified.


## Irancell paired follow-up — 2026-10-14

The user switched the same phone to Irancell. The target-observed source prefix was independently associated with **AS44244 (Iran Cell)** using RIPEstat; the phone's dual-SIM display alone was not treated as proof of the active data route. The destination and production configuration were unchanged, and both control and phone used official Xray **26.5.9**.

| Direct test | Off-carrier control | Irancell phone |
|---|---:|---:|
| Same-port plaintext HTTP / TLS 1.3, two runs per size and direction | 12/12 | 12/12 |
| Ordinary VLESS / TLS 1.3: 12 directional/tiny cases plus 3 combined bulk transfers | 15/15 | 15/15 |
| Existing production REALITY on 443: two 204 checks and two complete robots.txt fetches | 4/4 | 4/4 |
| **Total** | **31/31** | **31/31** |

All eight same-port bulk cases that failed on MCI completed on Irancell. All three combined VLESS cases verified the full 16 KiB upload and 64 KiB download, including the nonce and server-computed upload hash. All **54 synthetic requests** across both sides correlated with origin request/response logs; no partial-body success was accepted. The production REALITY responses matched the off-carrier control body hashes, including both complete 6,624-byte robots.txt bodies. Production REALITY used its existing credentials, SNI and chrome fingerprint, without FinalMask.

These results establish that the direct target and fixed official core can transfer complete payloads over the tested Irancell session. They strengthen the path-dependent diagnosis for the prior MCI failures. They do **not** identify the DPI mechanism or prove a permanent carrier-specific block: the measurements were sequential, and route/time changes remain possible. The synthetic fixture reused the same target, ports and methodology with fresh short-lived certificate/UUID material after the earlier cleanup; it was not byte-identical cryptographic material. The production REALITY configuration remained unchanged.

The four production phone requests took approximately 1.37–2.06 seconds end to end. These small transfers are not a useful-throughput benchmark, and occasional longer synthetic requests were observed. This is sidecar protocol acceptance, not official v2rayNG import/TUN, long-running browsing, screen-off or reconnect acceptance. No v2rayNG application or embedded-core changes and no extra traffic relay were introduced.

Cleanup was verified: all four temporary units stopped, all five test ports closed, target and manager fixture credentials removed, and no phone sidecar process remained. Main 443 remained listening and the production configuration hash matched the pre-test hash. Header-only packet metadata and origin logs were archived through the existing guarded transport and archive SHA-256 was verified; this still does not prove phone-side packet reception. Remote phone services remained running with autostart enabled.

**API usage for this follow-up: 0 requests, 0 tokens, USD 0 additional estimated cost.** The preceding two-request Astra stage remains 21,181 tokens and USD 0.9922925 estimated; this is not an account-wide usage total. The follow-up changes documentation and private receipts only, so the prior code acceptance results above remain applicable and no full code test suite was repeated.

Next: switch the same phone back to MCI for a bounded A-B-A confirmation. Keep direct routing, the fixed client, production safety and lifecycle guards intact. Do not promote or deploy fleet-wide on the strength of Irancell success alone.
