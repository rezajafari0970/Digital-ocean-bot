# Native provider browsing

The default provider button opens a separate setup page for a temporary V2RayNG
custom profile. Select the proxy and ordered countries, create the connection,
import the JSON as a custom configuration, connect and run a real connection test.
After actual upstream response bytes have passed through the tunnel, the page
unlocks the original website link. Chrome performs all rendering and interaction.
Use Chrome menu / Desktop site for desktop layout.

For separate account cookies, close every existing Chrome Incognito tab and open
a fresh private session using Copy website address. The ordinary Open here button
uses that browser's existing cookies. Panel JavaScript cannot force Incognito or
Android Always-on VPN / Block connections without VPN; configure those on-device.

The 15-minute connection is independent of whether the setup page is visible.
Logout, account cleanup, invalid route identity, expiry or End connection revoke
active streams. A restart ends all temporary sessions. Import a fresh profile to
continue. Upstream proxy master credentials never leave the server.

Native clients use authenticated VLESS v0 TCP over WSS on the existing HTTPS panel
endpoint. WebSocket Upgrade/Connection headers are normally forwarded by the edge. For
the existing loopback Nginx gateway that strips those hop-by-hop headers, the
endpoint can reconstruct a valid version-13 handshake from its preserved
Sec-WebSocket nonce. Reconstruction is restricted to loopback peers, no Origin,
and a valid 16-byte nonce; it never grants upstream access without the session
UUID. A real Nginx fixture intentionally strips these headers and verifies full
HTTP/HTTPS payloads and revocation through the resulting tunnel.
The endpoint accepts only temporary session UUIDs and public TCP ports 80/443,
rejects browser Origin, and reuses strict country/IP-pinned onboarding egress.
No direct provider fallback exists. DNS runs through in-tunnel DoH; UDP/QUIC and
non-web traffic are blocked. Only the panel's exact endpoint is direct so users
can control the connection while it is active. Chrome can be selected alone in
V2RayNG per-app mode; the profile is not a general-purpose VPN replacement.

The supplied profile uses the SOCKS inbound used by Android VPN traffic. Xray's
HTTP proxy inbound was excluded after a close-delimited response test exposed
an HTTP-parser/closed-pipe failure. Regression fixtures use a real pinned Xray
client, HTTP/HTTPS payloads including 240 KB, upstream close and revocation.
Browser UI fixtures use disposable certificates and synthetic accounts only.

Native Chrome compatibility does not guarantee challenge-free access. Provider
policies, IP reputation, proxy quality and session history can still cause
Cloudflare/CAPTCHA checks. No CAPTCHA automation or fingerprint disguise is used.
The remote renderer remains available explicitly with mode: remote; its tests do
not certify physical Android input, provider login or native Chrome acceptance.
