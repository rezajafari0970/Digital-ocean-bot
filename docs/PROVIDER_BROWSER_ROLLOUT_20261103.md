# Provider browser rollout — 2026-11-03

User scope: three AUTO-default country priorities during account creation and an
isolated provider browser through the selected countries, available from the
account wizard and each account. No cloud resource was created for testing, no
saved country policy was changed, and uncertain Kamatera creates were not replayed.

## Current status

| Stage | Evidence |
| --- | --- |
| Authorized storage cleanup | Complete; final receipt reconciled in STORAGE_CLEANUP_20261031.md |
| Final deployment | Both live instances run 9b2c75e at schema 206; default verified 12:34:48 UTC, production 12:51:57 UTC |
| Preservation | All 18 checks passed per instance; exact running executables and 22 served assets matched |
| Production identities | All 2,267 profile identities and ten Trojan identities in the correction baseline unchanged |
| Output routes | Eight raw routes and four viewers passed; positive samples returned 40 global and 20 account URI lines per class |
| Actual provider browser | Two isolated Italy/Vultr sessions opened; both rendered the real site's Cloudflare security challenge; authenticated login was not tested |
| Correction source CI | PR36 merged as b60688c; run 41634988073 passed all 353 mandatory roots, zero skips |
| Independent correction installation | Candidate 9b2c75e / run 41635633385 passed test, fresh installation and repeat installation |
| Correction deployment and live acceptance | Atomic promotion 41636644910 passed; all 11 rollout/acceptance steps completed successfully by 12:52:22 UTC |
| Protected backup vault | Port 18443 active at unchanged PID 1176, zero restarts; unauthenticated HTTPS correctly returns 401 |

## Live proxy regression and correction

The first actual Italy/Vultr browser launch returned `browser_route_unverified`.
The temporary administrator was removed; no website session or direct fallback
was opened. The sole configured proxy uses the suffix-session driver and SOCKS
port 824. Three independent proxied observations with `sid` produced three
separate IPs, while the documented `sessid` parameter retained one Italy IP on
all three connections. Saved proxy/account configuration stayed unchanged.

The correction adds an optional ephemeral-session driver contract. Onboarding
proofs and all subsequent provider/browser connections use its `sessid` grammar.
Existing dedicated-port account identities retain their original grammar. The
native CONNECT/TLS regression requires the appropriate grammar in both modes,
including proof, provider dispatch and country-drift rejection. The driver unit
regression also covers AUTO, existing suffix replacement, and fixed/generic
adapters. The guards still reject missing or changed country/IP evidence.

Primary driver reference: https://docs.dataimpulse.com/proxies/parameters/session-id

- Source PR: https://github.com/rezajafari0970/Digital-ocean-bot/pull/36
- Exact source: `c6d7f756dcb5a6d2e3e037d1c152d9878cbf60cd`.
- Exact public candidate: `9b2c75efe8399c4aade0bbcc55eb3858f5c5a3b6`.
- Export: 1,869 files; all bytes and executable modes match the immutable source export.
- FILES.json SHA256: `5fdbb695c1fb78bd708dd3fd6d31c0e21c00698ab5b50d393ab7b184e9ec4096`.
- Both live-secret scans and configured gitleaks passed the exact candidate.
- Public main/stable were atomically promoted to `9b2c75efe8399c4aade0bbcc55eb3858f5c5a3b6` by run 41636644910 after both exact-commit CI gates passed; both refs were re-read.

Private correction receipts, clean source/export/public checkouts and reviewed
bounded rollout helpers are under
`/root/dob-provider-browser-20261103/ephemeral-fix` on `limited-lime.ptr.network`.
The completed `rollout-and-accept.py` verified that both public refs matched the
exact candidate. It captured fresh independent baselines, updated default then
production with existing installer guards, verified preservation/assets, checked
outputs, opened two Italy/Vultr browser sessions, and checked final health.
`continuation.json` records `passed`; no further updater should be started for
this rollout.

## Final live acceptance

The correction default update completed at 12:34:46 UTC and production at
12:51:53 UTC. The native production backup completed a full custom-format parse,
sealed source `20261103T125022.272049Z`, and passed encrypted archive integrity
checks at 12:51:40. Both instances retained all 18 policy/secret/identity checks,
all 22 served assets matched the approved export, and all six running executable
hashes matched their release manifests. Production retained all 2,267 profile and
ten Trojan identities from its fresh correction baseline with zero changes.

At 12:52:22 UTC, both instances passed all five readiness checks, all six services
were active with zero automatic restarts, no deployment was pending, and
25,614,512,128 bytes were free on the root filesystem. The protected port-18443
vault remained at PID 1176 with zero restarts and returned the expected
unauthenticated HTTPS 401. Temporary test sessions and the temporary operator
were removed. The task's temporary monitor was stopped after acceptance.

The actual-site acceptance opened two separate 390 × 800 Vultr sessions through
verified Italian proxy routes. Both used nonroot browser workers with distinct
network and mount namespaces. Unauthenticated start and missing-proxy requests
were rejected. Seven registered provider entrypoints were present. Saved account
country policy and credentials were unchanged; no cloud resource was created.

Both retained frames were visually inspected: they show the real
`https://my.vultr.com` Cloudflare security-verification page in Italian, with no
blank frame or browser error. This verifies selected-country access and real-site
rendering; it does not claim CAPTCHA completion or an authenticated provider
login. No challenge was bypassed. Native CI separately verified cookie/storage
separation and zero direct connections under its transport fixtures.

The final output sample passed eight raw routes and four public viewers over
loopback and the public HTTPS gateway. Positive samples contained 40 global and
20 account URI lines per route class, with no withheld entries or control-panel
HTML and with `Cache-Control: no-store`.

To use the feature, open Accounts → New, set the three country priorities (all
default to AUTO), choose an assigned proxy, and open the provider browser from
the wizard. Existing accounts expose the same provider-browser action. Explicit
countries are tried in priority order; if none can be verified, the browser
refuses to open instead of connecting directly or using an arbitrary country.

## Completed original deployment and recovery

The default instance completed installation at 10:59 UTC. Its actual installed
Chromium returned an isolated 390 × 800 frame while all tunnel attempts were
denied. Source CI 41124609102, public CI 41124939606 and atomic promotion
41125845769 had already verified the original feature release independently.

Production initially reported `database_backup_unavailable` because its 06:00
pg_dump exceeded 1,800 seconds. Native database integrity itself passed. A new
sealed backup and encrypted archive were obtained before the first swap.
The first migration hit its original 15-minute deadline during the full retained
provider-snapshot comparison. The transaction rolled back to schema 205 and the
old runtime. The resource monitor requested no cancellation. Startup of the
restored runtime waited for its integrity/backup dependency and exceeded the old
five-minute readiness wait. At 11:41:53 UTC, the already-completed rollback was
reconciled under installer locks only after exact runtime, configuration/key,
instance identity, sealed backup/archive and all five readiness checks passed.

The reviewed production operator correction uses session-only
`default_toast_compression=lz4` for temporary migration row copies, Zstandard
level 1 for new custom-format backups, and a bounded 15-minute readiness wait
including native startup work. Migration still has its original 15-minute
limit and every retained-row/column/sequence comparison. No global PostgreSQL
configuration, retained data, backup validity requirement or disk guard changed.
Four transaction/rollback tests passed. A 100-row bidirectional EXCEPT ALL sample
was identical: pglz copy 1105.382 ms versus LZ4 294.196 ms; this is not a general
whole-database performance guarantee.

The retry's full new Zstandard backup sealed source `20261103T115545.082183Z`;
its archive completed at 11:56:29 UTC. Migration began at 11:56:30 and passed all
retention checks by 12:08:12; schema 206 committed. Peak backend RSS was
958,959,616 bytes and minimum available host memory 1,123,315,712 bytes, with no
cancellation. The post-migration backup sealed source `20261103T121132.389707Z`
and matching archive completed at 12:11:54. All five readiness checks and 18
preservation checks passed at 12:12:14. Six services had matching executable
hashes and zero restarts; no pending deployment remained.

PostgreSQL references:
- https://www.postgresql.org/docs/18/runtime-config-client.html
- https://www.postgresql.org/docs/18/app-pgdump.html

## Output sampling and browser limits

The first output check selected the newest account capabilities, both belonging
to DigitalOcean accounts whose servers and output rows had expired. Their empty
HTTP 200/no-store output was correct. The corrected operational sample retained
those observations and selected existing capabilities with actual publishable
rows; eight raw routes and four viewers passed with no withheld entries,
redirects or control-panel HTML. No share token or output record was created.

Each browser session has its own disposable cookie/storage profile and nonroot
network/mount namespace. Native CI covers HTTPS, WebSockets, input, cookie
separation and blocked direct routes. Real provider-site acceptance is recorded
separately and must not be inferred from an isolated blank frame. Sessions last
at most 15 minutes, expire after two minutes without control activity, and do not
support uploads/downloads. Country priorities are tried across the assigned
eligible proxies; AUTO slots beside explicit countries never add arbitrary
fallback. These checks do not guarantee every third-party login or CAPTCHA flow.
