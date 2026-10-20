# Cloudflare TLS control plane — 2026-10-20

Implementation branch: `feat/cloudflare-tls-20261020`. Baseline: `4223cea404afa2ab5620cb24306f93e60fe7b4ac`. Migration: `000189_cloudflare_tls`.

## Scope and compatibility

Cloudflare is an optional ingress module. Existing profiles have no Cloudflare account field and retain their original behavior. The first supported combination is VLESS/VMess/Trojan over WebSocket + TLS on the standard Cloudflare HTTPS ports. Raw TCP, REALITY, gRPC and XHTTP are not admitted by this first CDN driver. The existing Google Ads selective residential egress remains unchanged. No Android application changes or insecure TLS option are required.

The module separates DNS, certificate and IP source interfaces, with injected factories for testing and future drivers. PostgreSQL owns scheduling, record ownership, immutable random lifetimes, retry times and ambiguous-operation state. The panel control plane retains its existing native mutation journal and client ownership. DNS rotation does not regenerate client credentials.

## User workflow

1. Open **Configurations → Cloudflare domains → Add Cloudflare account**. A scoped API token is preferred; Global API key plus email is also accepted. Credentials are write-only and encrypted using the controller's versioned keyring and account-bound authenticated encryption.
2. Save the credential to retrieve full-setup zones, then select active domains. The token needs Zone Read, DNS Edit, SSL and Certificates Edit, and Zone Settings Read for those domains. If automatic Full (strict) is selected, Zone Settings Edit is also needed. Full (strict) is zone-wide; the UI states this explicitly.
3. Create a **WS + TLS** profile and enable **Automatically use active Cloudflare domains**. Select the account, record count, interval, lifetime range and active-record ceiling. Automatic port allocation only considers Cloudflare-compatible HTTPS ports and retains the existing collision checks.
4. Records and certificates prepare independently. Native listener observation, current routing/class proof, a real authenticated connection through the chosen CDN endpoint, strict public TLS, current profile revision and owned record evidence are all required before export.
5. Refresh subscriptions before old hostnames expire. Static links cannot transparently learn a replacement hostname. Deletion does not evict resolver caches immediately, and an API outage can delay remote deletion; expired records are withdrawn from publication immediately.

Defaults: one record per server every 30 minutes, independently randomized 45–60 minute lifetime, maximum four active records. Limits: one or two records per batch, up to eight active records per profile/server, 500 owned active records per zone. Capacity must cover overlapping generations. Lifetimes are capped by server expiry. DNS TTL uses Cloudflare's automatic proxied TTL and is separate from the record lifecycle.

Names use a single DNS label such as `assets-stream-delivery-<128-bit random hex>.example.com`. No Unicode tricks or invalid characters are generated. The official full-setup Universal SSL first-level coverage is respected.

## TLS and recovery

Each server generates its private key locally under `/opt/dob-cloudflare/<account>/`. Only a public CSR goes to the controller and Cloudflare Origin CA. The controller verifies the certificate chain against the checked-in official Cloudflare RSA root, CSR public key, SAN coverage and expiry. Certificate installation uses an atomic generation symlink and verifies the local key pair. No Cloudflare credential is sent to the server.

Origin CA secures Cloudflare-to-origin only; it is never exported as a directly trusted public endpoint. Clients verify Cloudflare's public edge certificate normally. The origin record contains the origin server IP, while the client address may be the owned hostname or a tested Cloudflare edge IP. SNI and WebSocket Host always contain the owned hostname.

Certificates have a seven-day requested validity and renew with a 48-hour margin. An installed certificate is inspected on reconciliation; missing or mismatched local files initiate a new generation. A lost issuance response is resolved by CSR identity. After two spaced negative provider observations and a five-minute quiet period, a fresh CSR/key intent may be queued, preserving the ambiguous intent in history. This recovery is limited to three fresh intents per account/server/day; an ambiguous POST is never replayed blindly.

DNS creation is journaled before POST. Lost responses are observed by exact hostname and ownership marker. Known rejected requests can retry; ambiguous writes remain observation-only. Deletion requires exact owner marker, hostname, type and origin content and is confirmed by a subsequent read. DNS cleanup ownership survives deletion of an ephemeral panel. Foreign or conflicting records are left intact with an explicit conflict state.

Per-record and per-zone retries do not block unrelated resources. The aggregate API budget is approximately 857 requests per five minutes, below Cloudflare's documented general 1200-request limit; endpoint-specific 429 responses still impose their Retry-After cooldown. Full (strict) observations expire after ten minutes and are refreshed independently. The existing worker budget also bounds this module.

## Iranian operator IP measurements

The official source is `https://api.cloudflare.com/client/v4/ips`, with ownership documentation at `https://www.cloudflare.com/ips/`. An official range is not evidence that every IP is reachable or fast on MCI, Irancell or another Iranian network.

The controller refreshes ranges daily and keeps the previous valid list on source failure. It accepts operator-reported results only through an authenticated administrative operation, within official ranges, with a successful protocol probe and a measurement no older than one hour. These are explicitly operator-reported observations, not an independently attested ISP identity.

From a ready record in **Cloudflare domains**, download a probe target. It contains one already-published config and official candidate ranges. On a machine actually using the desired operator, with the official Xray core installed, run:

```bash
python3 tools/cloudflare/operator-probe.py --target-file cloudflare-probe-ID.json --operator mci --xray /path/to/xray > operator-results.json
```

Import `operator-results.json` from the same panel. The probe validates TLS and fetches a normal HTTPS page through the actual WS+TLS config; it does not request advertisements. The target file contains ordinary client configuration credentials; no Cloudflare token or controller administrator token is needed for this download/run/import workflow. An optional authenticated direct-submit mode is available for an operator-managed probe host.

An operator preference uses a fresh matching-zone measurement when creating a record. Without one, it falls back to the owned DNS hostname. The selected address is immutable for that record and must still pass the controller's authenticated CDN traffic proof before publication. Datacenter measurements do not establish Iranian operator reachability. No 99% load rate or absolute speed claim is made.

## Validation and rollout status

- Implemented: provider interfaces, encrypted accounts, zone selection, record scheduling, certificate provisioning, profile toggle, strict CDN export gates, official IP ranges and operator probe workflow.
- Fixture/native validation: 18 focused race/database/native/OpenSSL tests and 12 browser scenarios passed; individual results are retained in private task evidence. The publication gate includes the Cloudflare tests and refuses skipped required cases.
- Production rollout: pending exact-candidate publication and installation checks. Feature defaults remain off.
- Live Cloudflare acceptance: pending a user-supplied account, selected active zone and one canary profile. No live DNS or certificate was created during implementation.
- Iranian operator acceptance: pending measurements from actual operator networks.
- OpenAI API usage: zero.

Official reference material consulted: Cloudflare API zones/list, DNS records/create, Origin CA certificates/create, Origin CA documentation, supported proxy ports, WebSockets, Universal SSL limitations, DNS TTL and API rate limits.
