# Native provider browser release — 2026-11-06

Accepted at 2026-11-06T18:17:46.321922+00:00. The user selected native phone Chrome after the new recording showed the remote viewport shrinking around the keyboard and a Cloudflare challenge continuing without successful login. This release changes the default provider action to native-client setup; it does not certify the former remote renderer as equivalent to a phone browser.

## Release identity and acceptance
- Tested source 026bd20c207296e308ae95e52bbfd518a72c3bfc, merged as 5ca143ae54594fa74eef48688e8b11612ca96268 in PR47.
- Public main/stable and BOTH runtime instances: d1c4a95df21a1fb10ada7ed2286cfab3cfbf6d20; schema206 unchanged.
- Source CI 42266285787: 377 mandatory roots, zero skips; native/browser/race/database/restore passed. The focused Chrome UI run 42266285801 also passed.
- Public CI 42266620205: tests, exported manifest and Ubuntu fresh/repeated installation passed. Atomic promotion 42267840951 rechecked the exact candidate and both destination leases.
- Both bounded updates completed with native integrity, backup and disk checks. Eighteen preservation comparisons and five readiness checks passed per instance. All six running binary hashes matched. HTML and32 assets matched both installations and the normal-TLS public gateway.
- Public acceptance used a temporary operator and real Xray through the existing HTTPS address: native session creation with explicit AR policy, WebSocket upgrade, untouched website TLS, the same pinned egress IP, UDP DNS redirected into in-tunnel DoH, real response-byte accounting and revocation without direct fallback. The temporary operator, client process and exported test credentials were removed. No provider login or cloud resource mutation was submitted.

## Native behavior
The provider button opens a separate setup page. The user chooses proxy/country priorities, imports a temporary JSON as a V2RayNG custom configuration, connects and runs a real connection test, then opens the original website in Chrome. Display, keyboard, clipboard, touch, scrolling, file controls and the Desktop site option belong to the native browser and original website.

The service retains upstream proxy master credentials. Only a random temporary VLESS credential leaves the server. The 15-minute WSS/TCP connection uses the strict pinned onboarding route; logout, expiry, account cleanup, invalid authorization, changed proxy identity or End connection close active streams. The transport is bounded and authenticated before upstream egress. DNS uses in-tunnel DoH, unsupported UDP/QUIC and non-web traffic are blocked, and only the panel's exact endpoint has a direct routing rule for control access. This is a dedicated browsing profile, not a replacement for the user's general VPN configuration.

Actual upstream response bytes are required before the setup page reports that a client response was received. That is historical evidence, not a reading of Android's current VPN switch; the page says so. Android Always-on VPN / Block connections without VPN are device settings. For separate account cookies, close every Chrome Incognito tab and start a fresh private session. The ordinary Open here button uses current cookies. Panel JavaScript cannot force Incognito or erase a provider's native cookies.

## Reproduced integration defects
The real public gateway forwards HTTP/1.1 but strips Upgrade and Connection headers. A narrowly filtered, unauthenticated probe established that behavior without capturing unrelated requests. The application reconstructs the version13 handshake only for loopback peers with no Origin, a valid 16-byte Sec-WebSocket nonce and compatible upgrade metadata. This does not grant provider egress: the temporary session path and UUID are still required. Real Nginx tests intentionally strip both headers and verify HTTP/HTTPS payloads, large responses and revocation. No gateway configuration or quarantined primary service was changed.

Real Xray testing also exposed a close-delimited response failure in Xray's separate HTTP proxy inbound. The supplied Android profile uses its tested SOCKS inbound. Coverage includes repeated server-close responses and 240KB HTTP/HTTPS payloads. No artificial relay delay or direct fallback was used to hide that failure.

The UI preserves clipboard success/error feedback instead of overwriting it on every status poll. Failed asynchronous clipboard access has a manual copy fallback. Late status responses cannot revive an ended session. The focused browser fixture reads the real clipboard, follows the original website tab, checks mobile layout and rejects stale status after End.

## Continuation limits
Physical Android acceptance, actual provider login and solving a security challenge were not performed. Native Chrome does not guarantee challenge-free access; no CAPTCHA automation, stealth scripts or fingerprint disguise was introduced. The user's selected proxy quality and the site's own decisions still matter. The old remote renderer remains explicit via mode: remote, with its previous documented limits.

All deployment intents in this completed release are final: do not replay updates. Preserve the original-cluster quarantine, credential/country policies, listener ownership and unknown-create receipts; do not replay unknown cloud creates. Earlier cleanup-statistics, bulk local cleanup removal and provider error-history changes remain included. No live bulk purge was executed here. Do not run heavy or high-DPI browser tests on the4GB no-swap runtime host; disposable CI owns those tests. Private evidence: /root/dob-native-browser-20261106.
