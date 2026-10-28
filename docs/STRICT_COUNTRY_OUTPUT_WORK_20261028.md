> Completed rollout: [Strict country and output release](STRICT_COUNTRY_OUTPUT_RELEASE_20261029.md). Historical work notes below retain their original stage status.

# Strict country onboarding and live output repair — 2026-10-28

Status: implementation complete; final gates active; no new release or runtime mutation yet.
Read the paired JSON for stage status and video timestamps.

The user supplied a new 154-second Android recording and a country-editor
screenshot. Review covered the complete recording at two-second intervals and
the output transition at half-second intervals. Its audio track is effectively
silent. Public documentation must not include personal screen contents, native
share tokens, API secrets or configuration URIs.

New live reproduction: existing global and account-scoped DIRECT/RESIDENTIAL
capabilities return HTTP 500 for raw output through both localhost and the
HTTPS gateway, while their HTML viewers return HTTP 200. This is an application
failure and was not covered by the previous service readiness/assets checks.
There was no cross-origin redirect in the direct probes. cPanel appears in the
recording after a navigation/back transition; an application redirect has not
been established. Diagnose before attributing its cause.

Country selection must be part of creation before provider discovery, default
to AUTO, and enforce an explicitly selected country across every eligible proxy
of that account without falling back to AUTO or direct egress. Preserve existing
isolation, uniqueness, snapshots, stale-worker fences and driver capabilities.

The three unknown Kamatera creates remain retained. Existing empty inventory/
queue observations are not terminal failure receipts and do not authorize replay.

## Current work locations

Local branch: `fix/video-strict-country-output-20261028` at base `ee595e75`.
Private runtime evidence: `/root/dob-strict-country-output-20261028` on
`limited-lime.ptr.network`. `live-output-before.json` proves the HTTP failures.
`diagnose-output.py` runs the native handler against read-only production SQL
in an isolated temporary source tree, with diagnostic stages and SQLSTATE only.
It never logs a raw URI, share token or database credential.

## Repair and verification checkpoint

The native independent-profile Trojan TCP HTTP query was rejected because the
publication allowlist omitted `headerType`. A full driver matrix additionally
reproduced rejection of Shadowsocks' native SIP002 HTTP plugin. Both contracts
now use shared transport validation. Invalid independent rows are withheld alone;
SQL, export policy, ownership, expiry and route-class failures still fail closed.

Read-only execution of the repaired native handlers on live production data
returned HTTP 200: 196 global ALL, 98 DIRECT, 98 RESIDENTIAL and six configs in
each sampled account class, with zero valid rows withheld. This is repair
validation, not a claim that the runtime has already been upgraded.

Onboarding and edits default to AUTO or exactly one country. Explicit routes
are verified before provider credentials are dispatched. Authorization intents
persist the country, account creation saves its constraint atomically, direct
mode is rejected for explicit policies, and replacement credentials use the
existing account's guarded route. A native one-connection SQL regression also
covers encrypted proxy credentials without nested pool acquisition.

Fourteen focused race/database/browser roots passed without skips. Full Go and
vet passed. The first comprehensive run passed 345 mandatory roots plus 20
recovery repetitions, then exposed an outdated account-module fixture missing
the country endpoint. That fixture and silent-country-change validation coverage
are repaired; the full gate is rerunning. Independent source CI, installation,
secret scans and live rollout remain pending.

The accepted API review request `strict1` ended with
`credit_balance_exhausted`; it returned neither review text nor usage. No
approval or zero-cost claim is made, and it was not replayed. Original Kamatera
journals show PROVIDER_UNAVAILABLE at all three unknown dispatches. No terminal
provider receipt has been found; the three retained creates are not replayed.

The second complete regression gate passed all 345 mandatory roots and 20 recovery repetitions, with zero skips. The repaired account-module browser fixture also passed. A final transaction-lifetime change releases onboarding network locks before local result persistence; it requires focused verification and exact-commit CI before publication.
