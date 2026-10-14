# Direct MCI investigation — 2026-10-14

Status: **A–B–A confirms MCI/main-destination path impairment in the measured window; no direct MCI remedy verified; experimental project changes only**.

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

- The Irancell and MCI return runs below complete the A-B-A comparison. Preserve the failure evidence; localize the affected path before further transport/kernel changes or promotion.
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
- **Updated external requirement:** both carrier runs and the MCI return are complete below. The MCI remedy, official v2rayNG acceptance, sustained-throughput improvement and fleet activation remain unverified.


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

The return-to-MCI comparison is completed below. Keep direct routing, the fixed client, production safety and lifecycle guards intact. Do not promote or deploy fleet-wide on the strength of Irancell success alone.


## Return to MCI and bounded UDP candidates — 2026-10-14

The target-observed return-path source was verified as **AS197207 (MCCI)**. The phone, its official Xray 26.5.9 binary/hash, target and production configuration were unchanged. MCI, Irancell and MCI-return observations now form a sequential A-B-A comparison; this is stronger than an unpaired report but does not eliminate all route/time or endpoint confounders.

| Main-destination batch | Off-carrier control | MCI return |
|---|---:|---:|
| Same-port HTTP/TLS 1.3, small and directional bulk | 12/12 | 4/12; only small responses |
| VLESS/TLS 1.3, directional plus combined bulk | 15/15 | 5/15; only small responses |
| Existing production REALITY/TCP 443 | 4/4 | 0/4 |
| VLESS/TLS 1.3 over mKCP/UDP, isolated high port | 7/7 | 0/7 |
| Same mKCP/TLS on UDP 443 | 2/2 | 0/2 |
| Same UDP 443 candidate with Salamander FinalMask | 2/2 | 0/2 |
| **Main-destination total** | **42/42** | **9/42; no usable profile** |

Both candidate configurations passed native validation with the unchanged client release and the server core. TLS certificate verification remained enabled. The UDP transport and mask trials modified isolated, bounded test services only; production TCP 443 was not changed. The probes used the same nonce/hash/full-body criteria as earlier tests.

All eight same-port bulk cases succeeded on Irancell and failed again on MCI. The repeated MCI plaintext downloads stopped at **15,207 of 65,634 body bytes**. A server capture of one such flow showed handshake and request completion, the initial response flight and retransmissions, with no subsequent client ACK observed at that server. This does not establish whether Android emitted those ACKs or where they disappeared.

A separate **direct synthetic reference** on the existing management host passed **6/6** from the same MCI phone, including complete HTTP/TLS upload and download. Local fixture controls also passed 6/6. This was a terminating diagnostic endpoint, not a traffic relay or a tested production proxy. Its different destination/route/OS limits causal isolation; it establishes that this phone/MCI session was not universally unable to transfer these payloads.

The high-port UDP packet capture reached its **6,000-packet limit** during the first phone attempt; later absent packets must not be inferred from that truncated observation. The UDP-443 capture contains 5,993 packet records for the observed phone source across the bounded trials. Initial phone UDP traffic reached the target and target replies were observed leaving it, but phone reception is not established. No tested UDP/mask candidate restored even the small synthetic request. No panic or fatal runtime error was present in the isolated core logs.

### Interpretation and next action

The supported diagnosis is **impairment correlated with MCI and the main destination/path in this measurement window**. Plaintext and multiple encrypted transports, the A-B-A result and the second-destination reference make a purely REALITY, SNI, certificate or generic phone-TLS explanation insufficient. They do not prove intentional DPI, identify a particular operator device or geographic segment, or exclude all provider/endpoint behavior. No setting tested here is a verified direct MCI solution.

Preserve the known Irancell result with its scope limits: four small production REALITY website responses do not constitute sustained production bulk/app qualification. A direct replacement origin on a separately verified reachable IP/prefix/provider route is an option that does not add a relay; it is not a repair or guarantee for the current IP. Provider/carrier route or source-dependent protection investigation remains appropriate.

The missing discriminating evidence is a synchronized observation closer to subscriber/access egress, ideally paired with provider-edge and target captures. Ordinary nonroot Android access cannot be assumed to capture the cellular wire. A rootless VPNService recorder can change TCP ownership and does not prove that the original socket's ACKs left the phone; two Android VPN services cannot ordinarily run together. A USB-tethered laptop can reproduce and capture its own flows, but must be labeled as a changed tethering/NAT/client scope, with source ASN/IP reverified.

Future output health must be keyed by observed carrier ASN, endpoint IP/port, transport, exact client version/hash, measurement scope and timestamp. Partial bodies/timeouts fail; failed controls make the result inconclusive; missing or stale evidence remains unqualified. Never average Irancell/control success into MCI health. A failed required sample should prevent promotion for that carrier/endpoint/scope, with fresh complete bidirectional samples and independent app acceptance required for recovery. These are acceptance requirements, not a newly deployed production quarantine system. The existing experimental assessment still returns **can_apply:false**.

### Cleanup, review and accounting

All temporary TCP/UDP listeners and the direct reference listener were verified closed; all corresponding test units were no longer running. The first UDP cleanup encountered an already-inactive transient unit; a fresh state read preceded an idempotent cleanup, followed by port/key/hash checks. Temporary certificate private keys and test credentials/configurations on target/manager were removed; client runners removed private files and stopped their sidecars. Production JSON SHA-256 remained unchanged and main TCP 443 remained listening. Phone remote services remained running with autostart enabled. Evidence archives were copied through the existing lifecycle/pinned-host guard and verified by SHA-256.

One additional sanitized Responses API review used **gpt-6-astra**, high reasoning, store:false, standard tier. It completed in approximately 104 seconds and supported the scoped path-dependent interpretation, capture limitations and conservative promotion requirements; it did not produce or validate a working bypass.

| API scope | Requests | Input tokens | Output tokens | Total tokens | Estimated USD |
|---|---:|---:|---:|---:|---:|
| MCI-return review | 1 | 815 | 3,925 | 4,740 | 0.2044000 |
| All three Astra requests in this investigation | 3 | 2,562 | 23,359 | 25,921 | 1.1966925 |

The new response includes 3,071 reasoning tokens within its output total, with zero cached-input/cache-write tokens. Cost uses published standard rates and is an estimate, not a billed invoice or account-wide total. Previous ChatGPT usage and older Sol-stage requests are outside this table. No full source test suite was repeated for this documentation/private-evidence-only follow-up, and nothing was deployed fleet-wide.

Primary configuration references consulted: [Xray mKCP](https://xtls.github.io/en/config/transports/mkcp.html), [TLS](https://xtls.github.io/en/config/transports/tls.html), [FinalMask](https://xtls.github.io/en/config/transports/finalmask.html). Version compatibility was established by actual native/control tests, not inferred solely from current documentation. Pricing reference: [OpenAI GPT-6 Astra](https://developers.openai.com/api/docs/models/gpt-6-astra).

## Localization follow-up — 2026-10-14, 03:54 UTC onward

**Exact DPI identity and physical drop location remain unproven.** This stage re-analyzed already archived target metadata and validated a new own-socket measurement capability. It did not run a fresh target-path test or discover a working bypass.

A live database read found the same target READY, account enabled and provider ACTIVE, but expiry remained 2026-10-14T04:22:06.006935Z (07:52 Tehran). The existing operator's greater-than-40-minute lifetime precondition was false. A guarded read-only date operation at 03:54:41Z returned target_precondition_failed before remote execution. Neither the guard nor target lifetime was changed. No replacement destination was silently substituted.

### More precise observations from the stored MCI-return trace

The analysis correlated each direct same-port client result with its matching source IP/port in the server metadata. Both plaintext download attempts had the same pattern:

| Observation | Both plaintext download attempts |
|---|---:|
| HTTP body received by phone curl | 15,207 bytes |
| HTTP response header length from fixture | 61 bytes |
| Unique server TCP payload sequence coverage | 15,268 bytes |
| Effective payload MSS in the captured flow | 1,388 bytes |
| Highest client relative ACK visible at server | 1; no response payload acknowledged |
| Captured RST / zero-window records | 0 / 0 |

15,207 + 61 = 15,268 = 11 × 1,388. The initial send comprised ten MSS of data followed by one additional MSS, then repeated retransmission of the first range. This accounts for the observed partial body without assuming a fixed 15-KiB censorship threshold. Host offload aggregated some records; packet-record counts are not physical wire-packet counts.

The two plaintext uploads each reached only 8,328 unique request bytes (six MSS) at the server. Server ACKs advanced through relative 8,329, but phone receipt of those ACKs is unknown. The fixture later emitted FIN after its configured 18-second read timeout; that server-generated close is not evidence of an injected reset. Curl's earlier 8-second deadline is likewise a local diagnostic timeout, not proof of a network device's timer.

Across the twelve same-port MCI flows, no RST or zero-window record appeared in the server trace. TLS handshake completion was observed in the curl results even for failed bulk cases. These observations make “TLS handshake always blocked” and “an observed reset injection caused these failures” inaccurate descriptions. They do not prove that no resets existed at an unobserved point or identify DPI, firewall, NAT, radio, transit, or provider mechanisms.

The narrower supported description is: initial data reaches the opposite application/host, then forward progress/acknowledgement feedback fails on this observed MCI–destination path. For the plaintext downloads, phone application receipt of the initial server flight is established, while the server never observes ACK advancement for it. Android ACK creation and physical egress were not captured during those historical flows; the exact gap cannot be localized further from this archive.

The TCP trace recorded 1,443 packet records and zero kernel capture drops (1,448 reported received by filter). It is finite, server-side, header-only metadata. The saved text does not preserve enough TTL/checksum/payload information for vendor fingerprinting or a geographic-hop claim.

### Additional capability validated, not a target test

Added experimental tools/networkdiag/tcp_info_reader.c and tcp_info_reader.py. They read TCP_INFO only for owned sockets using the native C header layout and returned structure length. Four full-body loopback cases and invalid-descriptor rejection passed on the nonroot Android phone and manager. The phone's kernel returned 232 bytes; the manager returned 280. No hard-coded Python ABI offsets or privileged operations were used.

Termux raw packet socket creation failed with permission denied (errno 13). No raw packets were captured, no root was installed, and v2rayNG and its embedded core were untouched. The helper has no daemon, listener after self-test completion, arbitrary remote-target CLI, relay, or persistence/autostart change.

Reference Android common 6.1 source confirms that segs_out increments before queue_xmit. Consequently even new TCP_INFO samples cannot alone prove that an ACK physically left the modem. They can narrow the interval between TCP accounting and target capture when collected on the same fresh failing flow; they cannot retroactively fill that gap in previous tests.

### What is still required

1. The same target must first satisfy the existing lifecycle guard through the project's authorized lifecycle process. A new IP would be a new scope, not localization of this IP's historical failure.
2. On an eligible target, collect synchronized, nonce-correlated phone own-socket snapshots and a bounded target capture for a tiny control and one failing plaintext bulk flow in each direction.
3. To identify the actual loss segment, obtain observation at subscriber/access egress or a provider edge and compare it with the two endpoints. Server-root access is not access to the provider's transit edge. Absent traceroute replies do not by themselves locate a silent drop.
4. Naming a DPI vendor/model requires provider/device-side evidence or a separately validated distinguishing fingerprint. None is present here.

No new OpenAI API call was made: **0 requests, 0 tokens, USD 0 additional API cost**. The previous three Astra reviews remain 25,921 tokens and USD 1.1966925 estimated, not an invoice or account total. No fleet or production behavior was changed.
