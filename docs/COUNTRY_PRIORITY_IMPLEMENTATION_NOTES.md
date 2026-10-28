# Ordered account proxy selection

Candidate implementation; activation requires the release gates and runtime
preservation checks recorded in COUNTRY_PRIORITY_WORK_20261028.md.

## Policy and compatibility

Each account owns an independent revisioned list of ISO country codes (250 options,
including XK) and an optional final AUTO tier. No policy row means legacy AUTO.
Migration 204 does not alter existing selections or learned country preferences.
The separate country editor reports desired revision, applied revision, active
stage and verification status; concurrent edits return a conflict and retain drafts.

The selector considers only the account's enabled assigned proxy pool. Country is
ordered before provider. A healthy incumbent is preferred within the same country.
Drivers register at startup and declare country targeting, sticky sessions, fixed
egress and dedicated ports. The core has no vendor-specific credentials or API.
Unknown drivers cannot provide explicit-country proof; fixed endpoints require
live country and reconnect evidence. Generic AUTO retains existing behavior.

A sticky driver currently supplies a finite budget of six independent candidate
sessions per country; a fixed endpoint supplies one. This enumerates the eligible
configured candidate budget, not an opaque provider's entire residential inventory.
Interrupted or inconclusive probes never count as failure or authorize descent.
Checkpoints survive restart. Negative evidence lasts five minutes; the next search
is scheduled separately, 30 seconds after a completed sweep. This avoids starving
lower tiers when a sweep spans multiple 22-second keeper invocations.

## Publication and request safety

Snapshots bind proxy definitions and credential versions before any network send.
A replacement key cannot be read for an older endpoint snapshot. Candidate probes
use separate gateways and observe country, IPv4, timezone and stable reconnections.
Provisional sessions never overwrite a serving route. A higher-country failback
requires a spaced second observation while a healthy incumbent continues serving.

Publication takes account-proxy then account-route locks, rechecks policy/pool/
credential scope, worker fence, active route and transport epoch, and atomically
publishes the profile, complete identity and generation. Existing unique IPv4 and
/24 indexes arbitrate concurrent account claims. Definition/secret/pool mutations
invalidate pending work; old workers cannot publish explicit identities without a
current account fence or bounded legacy AUTO compatibility lease. Downgrade refuses
to discard explicit policies or unsupported registered driver intent.

Default AUTO executes the legacy keeper. AUTO reached after explicit-country
exhaustion uses the same learned preference and legacy failback, with a bounded
lease and final revalidation. It never enables direct routing. Credential rendering
starts with the raw username and applies driver capabilities once per connection.

Request admission checks country proof, route, identity, generation and epoch on
the SQL connection holding the route lock. The lock covers request upload and
response-body completion, including early HTTP/2 response headers. Passive health
observations execute after releasing that connection, avoiding pool self-deadlock.

## Settings and video fixes

Account and proxy saves preserve omitted fields, retain unavailable saved catalog
choices, reject invalid values explicitly, and use configuration revisions to
reject concurrent drafts. Account/proxy updates require the client
revision; older cached forms must reload and review their draft. Account creation and secret writes are transactional.
Renaming a proxy does not reset its learned identity; omitted passwords are never
written back from an earlier read. Account schedules change only when relevant
build settings change; installer-specific profile fields remain intact.

The UI distinguishes desired regions from actual server regions, worker checks
from actual provider dispatches, current routing from dated publication history,
and unknown creation dates from real timestamps. Pre-SSH deployments have valid
stage diagnostics. Mobile tables retain field labels. Unknown provider CREATE
operations remain journaled and unreplayed; provider capacity is never bypassed.

## Release requirements

Meaningful regressions include native CONNECT/TLS and fresh sockets, unknown geo
observations, stale credential reads before I/O, non-idempotent independent drivers,
interrupted long sweeps, candidate uniqueness, concurrent /24 claims, policy/route
fences, failback stability, single-connection streaming admission, and real Chrome
settings/country-editor conflict tests. The canonical gate forbids missing/skipped
required tests. Independent OpenAI API reviews and all failed/corrected test runs
are recorded in the work log. Source CI, public export scans, native installation
and both-runtime preservation checks must complete before reporting release success.

## Final admission and concurrency safeguards

Repository connection parameters and the route fingerprint share one SQL snapshot.
Provider dispatch requires that fingerprint plus current generation/epoch and ready
country policy under the held route connection. Native request-write completion and
response consumption jointly determine release; EOF does not cancel an upload.
Keeper work uses a durable per-account owner lease without holding a SQL pool slot
through network probes. Its work deadline precedes lease expiry; every route
publication checks the lease. Health publication also rejects newer passive evidence.
Country proof windows scale to the finite configured driver work, with a five-minute
minimum and 30 seconds per attempt/tier. This prevents large finite scans from
restarting forever; an indeterminate observer never counts as exhaustion.
