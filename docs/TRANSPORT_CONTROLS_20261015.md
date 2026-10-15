# Transport controls and verified Fragment runtime — 2026-10-15

This continuation supersedes the prior empty-Flow/disabled-FinalMask limitation for NEW physical listeners. Existing pins retain saved settings. No provider purchases, IP allocation, existing-listener conversion, or phone-app source change.

Panel: editable XHTTP host, mode (auto/stream-up/stream-one/packet-up), path length, Extra JSON, Flow, FinalMask JSON/activation/scope and Browser Dialer preference. Vision-udp443 uses separate per-listener VLESS Encryption keys (domain-separated HMAC/X25519); server wire flow is Vision and client flow is Vision-udp443. Output verifies observed native flow before exporting; it never fills a missing wire flow from the policy.

FinalMask TCP Fragment works on the server and client. Official Xray26.10.10 Fragment lacked CloseWrite, causing a recovered REALITY panic and stalled connection. The patch forwards half-close without bypassing masking. Native Sanaei3.8.5 preserves Fragment on save, runtime generation and migration only after verified installed-core capability. Published artifact5793567513420e536bdfca28464dbe831d13caa2 contains binaries, complete modified sources, patches, licenses and hashes; public immutable URLs and compressed/raw SHA256 verified. Runtime versions Xray26.10.10 dob-fm-0.1.0 and Sanaei3.8.5-dob-fm.1, linux/amd64 only. Other native versions/architectures fail closed.

Installer requires the active PostgreSQL panel-config lock plus host flock, derives required capabilities from actual uploaded payload, validates candidate and current config before mutation, journals prepared/installing/applied/rolled_back, fsyncs backups and verifies backup hashes before recovery. It checks exact running panel/core hashes across all Go OS-thread child lists, native health and unchanged identity/inbounds. Rollback replaces binaries only and preserves DB writes. Partial uploaded secret files are cleaned even after cancellation. A failed or recovered installation requires operator review before retry.

Browser Dialer can be selected/saved, but remains INACTIVE with REALITY. Official core bypasses browser for REALITY and a VLESS URI cannot enable the phone app preference. Actual activation requires a TLS endpoint and client-side enablement. No fake bd parameter or claim of activation. UDP/QUIC FinalMask sections remain saved but inactive for XHTTP Reality HTTP/2; effective TCP Fragment alone is applied/exported.

Evidence:
- Actual native-panel save/generated-core roundtrip, final patched server and official unmodified Xray26.9.9 client: HTTP and UDP443 pass with both FinalMasks and4096-byte path. All four XHTTP modes tested.
- Native Fragment half-close/race and persistence/migration cases pass.
- Real Ubuntu24 installation, idempotence, interrupted partial installation recovery, blocked retry, and failed-readiness rollback pass. Intervening SQLite settings survive rollback. Test exposed and fixed non-main-thread core process detection.
- Full isolated PostgreSQL plus Chromium project suite passes (adminapi415.364s), focused final race, vet and diff checks pass. Three actual OpenAI API reviews: R1 REVISE, R2/R3 PASS;102070 input+9726 output=111796 tokens. Actual bill unknown.
- Real native canary38446: verified install, create/readback and restart/readback. Official26.9.9 client passes both204 probes and both200 ad scripts after restart. Existing ads-only policy correctly blocks Cloudflare and a nonallowlisted UDP443 echo destination. This WAN check is NOT a positive WAN UDP443 or SDK-ad-display claim; UDP443 positive evidence is the isolated real-core/native-roundtrip test.
- Phone SSH tunnel127.0.0.1:48022 is disconnected; no fresh phone or mobile-operator acceptance claimed.

Controlplane publication, secondary/primary normal updates and new-default activation/output acceptance are pending at this source checkpoint. Keep additional-IP work paused.
