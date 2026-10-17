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

Implementation and pre-release validation complete. Standard immutable publication, both-host update and native Trojan output activation are pending at this checkpoint. The new policy is disabled by migration; activation requires the recorded canary verification before fleet scope.

Detailed, non-secret stage records: `docs/development-jobs/trojan-tcp-20261017.json`. Private operational receipts remain on the control host. No OpenAI model API was called (requests/tokens/cost: zero).

## Canary correction

The first two-server canary found that Trojan routeOnly=true violated the existing strict Residential destination-override requirement. Only the owned Trojan sniffing setting was corrected; native VLESS configurations were compared before/after and unchanged. Both protocols subsequently passed real Residential HTTPS probes on both canary servers (4/4), and Direct probes passed (2/2). The generated Trojan payload now enables destination override, observation rejects the old incompatible mode, and the cross-package generated-payload regression is mandatory in publication. Fleet activation remains off until the corrected release is installed.
