# REALITY target recovery — 20 October 2026

The open VLESS TCP REALITY 443 incident is closed by enabling the existing verified automatic target selection and recovery for profile `3a5372ee-3fce-4083-bad6-35a55dc3de67`. Its revision changed from 1 to 2 through the revision-checked profile API. Existing and future eligible servers now choose among `www.microsoft.com`, `www.cloudflare.com`, and `www.apple.com`, separately per server. No binary deployment was needed; installed release remains `8c488709ba27d12d99772636e302d93c0973863b`.

## Cause and scope

The prior incident's bounded diagnostics found the configured Microsoft target unreachable while ordinary TLS egress and the inbound TCP endpoint worked. That server expired and was deleted before this follow-up. Its removal is not counted as a repair, and the deeper network cause cannot be reconstructed from that diagnostic.

The actionable configuration weakness was manual SNI: the existing runtime recovery deliberately cannot replace a manual target. Switching this profile to automatic selection enables its existing recovery path after repeated total client traffic-proof failures. It discards the cached failed selection and prefers another verified candidate. If none qualifies, the listener remains unverified and retries. TLS certificate/chain validation, public-target checks, TLS 1.3 and HTTP/2 requirements remain enforced.

## Acceptance

- Existing SNI validation regression passed, including invalid-certificate, private-target, incompatible TLS/ALPN, and manual-mode preservation cases.
- Native target scans on three live servers verified all three configured candidates.
- An unpublished canary included `unreachable.invalid`. It selected a valid alternative, passed both Direct and Residential HTTPS connections on the first attempt, and was then deleted. Its listener is retired and its deletion completed.
- Six additional HTTPS connections through the production profile succeeded on the first attempt across three servers representing all three selected SNI names. These tests used actual existing client identities with strict TLS validation. No production traffic proof was forged.
- At 04:20:59 UTC (07:50:59 Tehran), all 72 eligible servers had the new profile revision, fresh native/runtime verification, automatic SNI selection and current verified Google Ads routing.
- At 04:20:39 UTC, all three profiles were ready: 144/144 links for Trojan 500, 144/144 for VLESS 443 and 144/144 for VLESS 8443. Output returned 432 links in 35 ms, with zero blocked or pending targets. All 144 exported port-443 SNI values matched their native listener settings.
- All 213 retained eligible listener identities and encrypted credential records matched the baseline. Existing profile fields outside the intended SNI configuration, and global creation/output/protection/Trojan policies, were preserved. All three controller services and all five readiness checks passed.

A sustained outage of a selected production target was not injected. The live canary demonstrates rejection of an unreachable candidate and successful alternative selection; the repeated-failure recovery behavior is established from the installed implementation. Point-in-time success does not guarantee that external targets or networks will never fail.

## Client use and rollback

Refresh the subscription or import newly generated links before reconnecting: the selected SNI is now server-specific and can change after recovery. UUIDs and key material are preserved. Official v2rayNG and its core are unchanged. Google Ads traffic continues through Residential; ordinary tunneled traffic continues through server egress.

Rollback uses the profile API with the latest revision and the original saved manual spec, retaining all other current profile fields. Do not replay an old revision or bypass verification.

Machine-readable summary: [reality-target-recovery-20261020.json](development-jobs/reality-target-recovery-20261020.json). External OpenAI API calls: 0.
