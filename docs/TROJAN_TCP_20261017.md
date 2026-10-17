# Trojan TCP alongside VLESS Reality

Task baseline: `0d6df15968f0682840ca1469ce908faccaf11971`.

## Behavior

Trojan TCP uses a separate, opt-in policy and native listener, initially port 8443. VLESS TCP Reality remains on its existing port 443 with existing credentials and transport settings. Both saved Reality profile ports and the global Reality policy reserve their ports in both directions. Native inventory and an operating-system bind check reject collisions; no existing service is stopped to acquire a port.

Each selected server receives one separate Trojan password per enabled DIRECT/RESIDENTIAL class. The native password, durable ownership and fresh routing proof must agree before output publication. Global, account and shared outputs retain their scopes. Country flags and names use the same provider-location mapping as VLESS. Trojan output contains TCP/TLS options only, with normal certificate verification.

The user selected automatic trusted IP TLS. The node issues a short-lived public IPv4 certificate through a separately owned, version/hash-pinned acme.sh installation. HTTP-01 requires free, reachable port 80. Private keys remain on the node. An hourly timer renews the certificate before expiry; Xray reloads certificate files without restarting existing Reality listeners. IPv6-only nodes and UpCloud trial port restrictions fail closed with a recorded reason. No self-signed or insecure fallback is published.

Creation, certificate work and native verification are bounded. Fast verification has a separate queue so long ACME requests cannot starve ready listeners. All operations share the existing worker connection/work budget and native configuration locks. Creation respects the global mutation gate and cleanup fence. Credentials and ownership are durable before a single native POST; uncertain replies are resolved by observing native state, never blindly repeating the mutation.

## Verification before release

- Targeted unit tests: Trojan export, native observation, identity/password drift, collision rejection, client-operation coexistence, class routing, API and worker modules passed.
- Pinned Xray dob-fm-0.2.2 / 26.10.10: real TLS/Trojan tunnel passed; wrong password and untrusted certificate rejected. Test-only local-origin routing does not alter production policy.
- Disposable PostgreSQL: 14 required race-enabled regressions passed with zero skips, including lost POST response/restart, near-expiry verification, output scope, country labels, port guards, cleanup and fleet coverage beyond 64.
- Browser: independent Trojan settings save, no Reality edits, existing transport controls passed.
- Output viewer: four tests passed, including mixed VLESS/Trojan links.
- Provisioning tests and seven publisher-gate verifier tests passed.
- Real canary: port 443 occupied by existing service; ports 80 and 8443 free. Public IP certificate issuance and controller PKIX/IP validation passed. Full public chain required a bounded 16 KiB structured SSH response; other SSH callers retain their 4 KiB default.

## Release status

At the initial implementation checkpoint, standard immutable publication, both-host update and native Trojan output activation were pending; subsequent outcomes are recorded below. The new policy is disabled by migration; activation requires the recorded canary verification before fleet scope.

Detailed, non-secret stage records: `docs/development-jobs/trojan-tcp-20261017.json`. Private operational receipts remain on the control host. No OpenAI model API was called (requests/tokens/cost: zero).

## Canary correction

The first two-server canary found that Trojan routeOnly=true violated the existing strict Residential destination-override requirement. Only the owned Trojan sniffing setting was corrected; native VLESS configurations were compared before/after and unchanged. Both protocols subsequently passed real Residential HTTPS probes on both canary servers (4/4), and Direct probes passed (2/2). The generated Trojan payload now enables destination override, observation rejects the old incompatible mode, and the cross-package generated-payload regression is mandatory in publication. Fleet activation remains off until the corrected release is installed.

## Corrected release and fleet activation

Corrected product source 7d8ce6198af30dae6906eed6c31d72a28751521f, public release ff9f7ab756ab1d239cf07e4b360afa191c2e66bb, schema 000183_trojan_tcp. Both hosts passed running-binary hash/readiness/preservation verification; all 516 retained Reality configuration pins were unchanged on the primary. The publisher passed all 15 mandatory regressions, complete Go tests/vet, installer checks and exact-candidate isolated installation. Fleet policy revision 4 was enabled at 2026-10-17T15:57:55Z. Generation is in progress; no complete-coverage claim is made at this checkpoint.


## Full-matrix route-proof capacity correction

Fleet expansion exposed a capacity limit in the existing full native routing proof: a second inbound doubles the matrix to 66 native requests. At 87 eligible servers, proof age p95 reached 62.39 seconds and ten otherwise fresh panels lacked a proof inside the unchanged 60-second publication limit. Expansion was held at 16:12:32 UTC, retaining the 56 already installed listeners.

The correction sends the exact same routeTest forms through one host-key-pinned SSH session and executes them sequentially over the node's loopback interface. Native Sanaei closes a shared gRPC client after each request, so per-panel concurrency remains one. Controller concurrency, fresh-proof deadlines, expected route validators, internal DNS/relay/pool checks and fail-closed output remain unchanged. At most eight SSH batches run concurrently. Credentials are encrypted stdin, never shell arguments or node files; destination, path, request count, size, duration and returned results are bounded. If local execution is unavailable, the original complete HTTP matrix runs.

Two independent live canaries checked all 66 native cases without any native mutation. Execution fell from 11,152 to 3,414 ms and from 15,596 to 3,337 ms. Both methods produced equivalent validated routes; automatically selected pool lanes were required to be exact members of the native loopback outbounds with the same network and content-addressed plan. Tests cover the full matrix, missing/mismatched results, cancellation, complete fallback, fixed read-only endpoint and pinned SSH stdin. The required publication gate now includes these tests. Rollout and resumed fleet coverage are pending this correction's immutable release.


## Route-proof correction deployed

Source f22c3d2e78b55a59586c0fc8eec93ae0cd0d560e was published as 01e8095b522b2ce806113ac5a706168c36290a84 after 18 mandatory no-skip regressions, full source checks and exact-candidate installation. Both hosts passed readiness and running hash verification. All 522 retained Reality pins and the held Trojan policy were unchanged. Three subsequent observations had zero fresh snapshots withheld for stale route proof; the final observation verified all 91 serving panels within 60 seconds (p95 37.19 seconds, maximum 40.13). Direct and Residential real HTTPS probes passed for both protocols on both canaries (8/8). Fleet expansion has resumed; complete fleet coverage is still being observed.


## Creation queue fairness

The final fleet tail exposed synchronized five-minute retries when several creators lost the shared global executor try-lock. Creation now joins a bounded FIFO queue on that same single executor after certificate work and before the native configuration lock. It does not increase concurrency or bypass policy, cleanup or admission checks. Cancellation releases even uncertain lock ownership. The mandatory lost-response/restart regression now additionally holds the executor, verifies Reality configuration remains readable, cancels a queued attempt, releases the executor and verifies exactly one successful native creation. All 18 required regressions passed with zero skips. Deployment of this preventive queue correction is pending at this checkpoint.


## Final production result — 2026-10-17 17:13 UTC

Completed on both control hosts. Final product source: d61c5fa1099ebce866860f9f959ef8718c003ac3; public main/stable release: b843c75eaa4648810f3b6227e88349a91b3f7422; schema: 000183_trojan_tcp. All running service hashes and readiness checks passed. All 527 retained Reality pins were unchanged, as were accounts, environment, protection, Reality creation and output policies.

- Fleet policy revision 6 is enabled on port 8443 for DIRECT and RESIDENTIAL. Existing Reality remains on 443.
- All 91 eligible Trojan listeners were VERIFIED, with zero recorded errors. Actual publicly trusted TLS/Trojan HTTPS traffic succeeded on all 91 current output servers using the pinned native client, normal certificate validation and no insecure fallback.
- DIRECT contained 91 Trojan and 91 VLESS server entries. RESIDENTIAL contained 91 Trojan and 90 VLESS entries in its existing client-class scope. ALL contained 182 Trojan links and 272 VLESS links across all 91 servers; existing client memberships were not rewritten to equalize counts.
- All 91 healthy servers appeared in ALL, DIRECT, RESIDENTIAL and their public share outputs. Ten global/shared/account surfaces had zero technical names, country mismatches, unknown countries or connection-byte mismatches. Maximum observed response time was 39.13 ms.
- Health at the final observation: 91 HEALTHY and two new servers still PROVISIONING. There were no degraded serving panels in that observation.
- Real Direct and Residential probes for both protocols passed on DigitalOcean, Linode and Vultr samples. After the route-proof release, these sampled checks passed 20/20, in addition to the complete 91-server Trojan Direct pass.
- Publication required all 18 named regressions and their selected subtests to pass without skips, followed by complete Go tests/vet, installer/browser checks, secret/manifest checks and exact-candidate isolated installation.

One Vultr panel showed intermittent slow route-proof behavior during rollout. Its 66 local native cases passed in 2,257 ms, while one diagnostic SSH invocation took 10,868 ms. Its recorded lifetime expired at 17:07:57 UTC and the existing lifecycle marked it deleted at 17:08:39, before the latency cause was conclusively resolved. No manual retirement or health override was performed. The experimental connection-budget change was reverted and was not released; full case counts, the 60-second publication freshness bound and concurrency limits remain in force.

Operational receipts are retained privately on the control hosts; no passwords, tokens or raw subscriptions are included in this report. OpenAI model API requests/tokens/cost remained zero.


## Continuation audit and bounded transport correction — 2026-10-17 19:54 UTC

Both control hosts still ran b843c75eaa4648810f3b6227e88349a91b3f7422 with ready services and zero restarts. The new observation must not be confused with the earlier 91-server success: ten DigitalOcean nodes now belong to an account that the provider reports as LOCKED. That external account restriction cannot be repaired by a project health override; the account owner must resolve it with the provider. Two independent Vultr panels also showed intermittent route-proof timeouts, one recovering automatically.

On the remaining current Vultr panel, every one of 66 local native route checks succeeded in 2,067 ms, but the complete authenticated SSH invocation took 11,017 ms. A separate stage observation measured TCP 235 ms, authentication 1,629 ms and session setup 3,466 ms. The previous ten-second end-to-end batch budget could expire even when the native matrix was correct. Unlike the earlier retired-node experiment, this correction was reproduced on a currently eligible node.

The batch now allows at most fifteen seconds end-to-end, including an eight-second handshake limit; node-side native execution still has its ten-second cap, the complete readiness operation retains thirty seconds, and publication still requires proof fresher than sixty seconds. All cases, validators, SSH host pins, encrypted stdin and concurrency bounds remain. A failed HTTP fallback now retains the fixed, credential-free batch failure stage while preserving the underlying error identity.

Targeted regressions passed, including the new fallback-failure case (deadline identity retained, no extra reads, no native mutation). The candidate was exercised against the affected live panel: all 66 native cases matched the original route semantics, taking 8,494 ms locally versus 16,988 ms for serial remote HTTP. No native configuration was changed during the comparison. Standard full publication gates and deployment are pending at this checkpoint. Ten output surfaces still had zero country/connection mismatches and covered every one of the 79 stable healthy servers at the initial continuation observation.
