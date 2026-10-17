# Config country display names — 2026-10-17

Status: accepted and installed on both service hosts. Product source 000b369d263eab2cfcb7822a6fd4289467b4b013; public release 53ec1c322d6b61d8e81f9370c8b6e2dd462fa2ee.

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

## Final acceptance

At 2026-10-17T03:01:34.253391+00:00 (06:31:34 Tehran), 91 healthy servers were covered in ALL (273 links), Direct (91) and Residential (91). Corresponding public shares matched. Ten actual global/share/account output surfaces had zero old technical names, incorrect countries, unknown country labels or connection-byte mismatches against the stored native-derived snapshot. Two other servers were genuinely provisioning. Current fleet countries include India, Singapore, Japan, United States, Poland, South Korea and Brazil; test fixtures cover all four provider adapters, including UpCloud which has no active live cohort in this observation.

Both hosts passed five readiness checks and all three running executable hashes, with zero automatic restarts. Accounts, environment, creation/output/protection policies and all 387 retained primary listener pins were preserved. The schema remains182. Public main and stable CI succeeded (38996831293 and38996831276). Normal publication passed the extended11-test disposable PostgreSQL/race gate, the ordinary full suite/vet/deploy/frontend checks and exact-candidate isolated installation. Existing country-free fixtures needed display expectation corrections; the broad output run reached30/31 and the final corrected boundary assertion passed in the fresh11-test gate. Failed runs remain recorded, not relabelled green. The new country integration test fails on the old output implementation and passes on the new one.

Primary installation was preceded by a failed disk preflight (5281MiB free versus5482MiB required), with running services preserved. Root's reproducible Go cache was cleared, then865026048bytes of publisher build cache were reclaimed under its lock; preflight passed before installation. No production backup or account data was deleted.

The pre-release fleet briefly had unverified routing observations and recovered on the old product before installation. During acceptance, one new panel still lacked its Residential profile; one bounded invocation of the existing current-policy reconciler completed it. No target, cadence, budget, eligibility, freshness, ownership or health guard was relaxed. Initial post-restart snapshot warm-up and all pre-existing deficits are retained in the job history. This is a measured acceptance window, not a perpetual uptime promise.

Refresh subscriptions to obtain the new names. Already saved standalone URIs must be imported again. Full sanitized receipt: docs/development-jobs/config-country-names-20261017.acceptance.json. OpenAI model API usage remains zero.
