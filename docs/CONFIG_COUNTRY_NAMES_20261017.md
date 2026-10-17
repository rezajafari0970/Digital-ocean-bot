# Config country display names — 2026-10-17

Status: implemented; database validation and deployment acceptance pending.

User request: replace the technical VLESS display name with the server country flag and English name, for example 🇩🇪 Germany. Server location is separate from account/proxy egress identity.

## Implementation

The shared output response reads the actual provider resource region from the latest account inventory, matched by provider resource ID. The creation profile is the fallback before the first observation. Region metadata comes from the same persisted provider observation. No GeoIP or provider request occurs while serving output; location work is materialized once per active droplet, not per client. The resulting display name replaces only the fragment after #. Stored snapshots, first_seen_at, listener managed keys, client IDs, routing, limits, transport, expiry and admission guards stay intact. Existing and newly built configurations use the same path, including global and account public shares, Direct and Residential.

The table supports all 249 ISO 3166-1 countries/territories from the system iso-codes dataset; the source SHA-256 is embedded in the generated file. Names use common English forms where ISO formal names are unwieldy (United Kingdom, United States, Iran, South/North Korea, Russia, Taiwan, Vietnam, Laos, Syria, Tanzania, Moldova, Venezuela, Bolivia, Brunei, Palestine and the two Congos). Kosovo is the explicit XK Unicode extension, not an ISO assignment. UK is normalized to GB. Invalid/missing metadata displays 🌐 Unknown country, never a guessed flag. Emoji rendering depends on the importing client's font.

Vultr and Linode retain the API country field in the common Region model. Legacy Vultr catalogs preserve the field in their adapter-generated city/country name. Linode uses known historical geographic IDs or country-prefixed IDs, never city/state suffixes: Los Angeles, CA and Atlanta, GA must remain United States. UpCloud public zones have country prefixes including uk-lon1. DigitalOcean uses exact documented datacenter IDs including the two legacy sites; an unrecognized future datacenter requires verified mapping, rather than prefix guessing.

Primary references checked 2026-10-17:
- https://docs.digitalocean.com/platform/regional-availability/
- https://techdocs.akamai.com/linode-api/reference/get-regions
- https://developers.upcloud.com/1.3/5-zones/

## Verification and stage history

See docs/development-jobs/config-country-names-20261017.json. Initial read-only live metadata query resolved all current regions in 51.66 ms including process/SQL wrapper overhead. This is a single observation, not a performance SLA. Tests cover ISO coverage, exact sample flags, invalid codes, legacy region ambiguities, byte-preserving TCP/XHTTP fragments, actual-region precedence and all output surfaces. The existing publisher recurrence gate is extended with the real database country-output test; no existing requirement is removed.

OpenAI model API usage: 0 requests, 0 tokens, 0 cost.
