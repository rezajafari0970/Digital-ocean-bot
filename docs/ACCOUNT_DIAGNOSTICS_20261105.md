# Account and service audit — 2026-11-05

## Stages

| Stage | Status |
| --- | --- |
| Recover deployed revision and prior cleanup | Complete: baseline was public 9b2c75e on both instances, schema 206; prior cleanup completed |
| Read-only service/account audit | Complete: six application services and protected vault active, zero restarts; both instances pass five readiness checks |
| Reproduce missing account diagnostics | Complete: independent observation details absent from dashboard; generic Refresh message hides provider detail; failed Refresh does not reload persisted state |
| Shared backend/UI correction | Complete and deployed; independent provider, observation and ambiguous-create issues visible in list and detail |
| Independent source/database/browser/race verification | Complete: exact source 2e07f096; 359 required roots passed, zero skipped; recovery repeated 20 times; backup restore passed at schema 206 |
| Initial diagnostics installation and guarded rollout | Complete on both instances; 18 preservation checks and 25 served assets verified per instance |
| Populated dashboard layout follow-up and final live acceptance | Complete: final a3ca806 on both instances; populated pages pass six widths under normal production CSP |

The local baseline exactly matches canonical source tree
`058cbdfc451ca7d7354bb024b097e61e32827b71` at source commit
`ffe2fe17f3885c868340131466edb5bd30fcc1f3`. The preexisting operational promotion
file matched its tracked canonical copy and was preserved.

## Actual provider findings

| Provider | Live evidence before correction |
| --- | --- |
| DigitalOcean | Three enabled accounts return an explicit provider lock. One historical deleted account is excluded from the active list. |
| Vultr | One enabled account: provider HTTP 401 authentication rejection. |
| Linode | Two accounts have successful scheduled inventory; one has authentication rejection. |
| Kamatera | Account and inventory succeed; three October 27–28 creates remain unknown, each dispatched once with neither resource nor task receipt. |
| OVH | Account and inventory succeed; one October 29 create remains unknown with no resource/task receipt, also dispatched once. |
| AWS | No saved account in either instance; no live credential acceptance claim. |
| UpCloud | No saved account in either instance; no live credential acceptance claim. |

The five restricted accounts also retain independent proxy-error evidence even
when their current proxy health is good. Successful network probes alone do not
prove a complete provider observation succeeded. Both errors and their original
timestamps must remain visible. All nine active account dashboards returned 200.

Private read-only baseline, API receipts and unknown-operation inventory are in
`/root/dob-account-audit-20261105` on the active controller. A temporary audit
administrator/session was removed in a finally block. Credentials, country
policies, schedules, provider restrictions and cloud operations were not edited.
The original controller remains offline/quarantined; port 18443 is protected.

## Correction and acceptance boundaries

One shared account diagnostic representation serves all seven providers. It
retains independent restrictions, unresolved observations and unknown creates,
including disabled accounts and cases where a desired target is already reserved.
Details parse worker JSON receipts, redact credentials/connection addresses,
bound output and escape HTML. Plain-text restriction evidence is no longer lost.
The UI preserves failed checks and provider detail, reloads persisted state after
failed Refresh/Preflight, and fences navigation to another account/page.
Runtime acquisition failures during Refresh now publish ordered observation
evidence. Preflight retains identity and creation-block errors separately.

Diagnostics do not grant create permission, clear restrictions or retry uncertain
cloud mutations. Provider locks require provider-side resolution; rejected
credentials must be repaired through the account form. Unknown creates require
authoritative provider evidence before releasing reservations. Paid cloud creates
and OpenAI generation requests made for this audit: zero.

Local syntax and focused JavaScript diagnostics tests passed. Chromium is absent
from the local workspace; actual browser acceptance passed in independent CI.
Mandatory regressions cover seven-provider list/detail parity, secret boundaries,
ambiguous-create immutability, runtime failures, simultaneous preflight failures,
failed-response detail, navigation fencing and real mobile/desktop rendering.

## Recovery deadline correction found by full validation

Source runs 42094675377 and 42095945874 failed the existing repeated recovery
isolation gate on the first iteration. Independent PostgreSQL logging in the
second run identified cancellation of `ClearResolvedChecked`'s correlated
three-branch DELETE, not a provider request. The subsequent operation scan
reported the already-expired context. The remaining 19 repetitions passed, so
neither a retry nor increased deadline is accepted as a correction.

Terminal evidence now joins the failure ledger once through UNION ALL branches.
The terminal/installer-selection predicates are unchanged. A dedicated regression
checks terminal and ongoing operations, ambiguous creates, live and deleted
servers, selected and legacy installers, and missing identities; retained errors
and retry timestamps must survive. The cold fixture also rejects JIT selection.
Error propagation now retains pruning failures alongside the stage that observes
the deadline. Both independent run failures remain part of the evidence.

A bounded, plan-only comparison on session-local temporary tables was rolled
back without executing either DELETE. Production data was compared with both
candidate selectors under one read-only snapshot; both selected zero rows and
EXCEPT ALL differences were zero. This empty live selection is limited evidence;
the nonempty semantic matrix is required in CI.

Fresh live Preflight checks confirmed all nine enabled accounts through their
assigned proxy routes. Both raw output classes returned 80 valid URI lines and
no-store. System counts were 20 active servers, 43 pending deletion/expiry, four
active builds and zero current failed provisioning runs. Historical diagnostic
families (46) are retained history, not 46 current installation failures.

## Exact independent evidence

Initial diagnostics source `2e07f09639342222b49e8271e0d561b6bef772f6` passed
[fleet run 42099293564](https://github.com/rezajafari0970/Digital-ocean-bot/actions/runs/42099293564),
job `127497530836`. The full Go/vet, database, native, race and browser gate
required 359 roots with zero skips. Recovery lifecycle isolation passed all
20 repetitions. Seven-provider account diagnostics passed actual 390/1365
mobile/desktop rendering, failed Refresh/Preflight readback and reload tests.
The independent restore drill passed all 206 migrations, encrypted identity and
configuration-key restoration, unknown-intent preservation, sequence validation,
amcheck, damaged-archive rejection, failed-restore atomicity and interruption
rollback, without accessing a live database.

The cold pruning fixture cost fell from 119844.47 to 17123.78; the new plan did
not select JIT. Source run `42097028044` already passed the 359-root gate, but
failed the final isolated JavaScript harness because its extracted API function
did not load the new shared message helper. The final source fixes that harness
and runs it early. Database-internal messages remain hidden from users.
The final complete run, rather than this partial earlier success, authorized release.

Public candidate `e2e6ba722760d28e4b7dd10625d7d1331ebe32b8` passed
[test and installation run 42099634706](https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/42099634706).
Test job `127498716794` and install job `127498716425` both succeeded.
Fresh Ubuntu 24.04 installation and repeated installation verified ordinary
login, all five readiness checks and retained identity. Both live-instance
secret scanners and configured gitleaks returned zero findings on the exact
export. Its 1874 manifest entries were verified; the FILES.json byte SHA-256 is
`45fdec50ca88d24fa32ad7e68f3501c34e3d7b2e3535d49dfe2d6d14f27e574e`.

[PR 37](https://github.com/rezajafari0970/Digital-ocean-bot/pull/37) merged as
`4fcc079235d55a6815e1f0f687d1e5be0873c069`, retaining the exact tested tree.
[Promotion run 42100787285](https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/42100787285)
rechecked the exact source/public CI evidence and manifest, then atomically
advanced main/stable from `9b2c75efe8399c4aade0bbcc55eb3858f5c5a3b6` with leases.
The operational workflow is preserved in
`ops/release/promote-verified-account-diagnostics-20261105.yml`.

## Populated dashboard finding during live acceptance

The first live acceptance stopped on layout, after successful installation,
preservation, API checks, all nine Preflight requests and both output classes.
At a physical 390px viewport the actual Vultr dashboard expanded to 417px,
causing automatic mobile zoom. At 1365px, its populated server table expanded
the desktop grid to 1646px. Empty server-table fixtures had missed this case.

Two CSS changes address the causes: `main` has `min-width:0` so the existing
table wrapper scrolls within the grid, and capacity values wrap inside statistic
cards. A temporary browser-only preview against the same live data confirmed
390px/390px mobile dimensions and a desktop document within the viewport.
This preview used temporary CSP bypass solely to inject diagnostic CSS and
changed no installed files. It is not final production acceptance.

[PR 38](https://github.com/rezajafari0970/Digital-ocean-bot/pull/38) adds ten
canonical server rows and a long capacity state to the actual browser regression.
It checks physical widths 320, 390, 800, 801, 1024 and 1365; mobile automatic zoom
cannot hide overflow by increasing `innerWidth`. Wide tables must scroll within
their wrapper. Final live acceptance used the installed candidate with normal
production CSP and no injected styles.

The follow-up source `7f46a0a7bc4b5f8ce9cd5f3a62e83589cb1579bd` passed
[fleet run 42104523666](https://github.com/rezajafari0970/Digital-ocean-bot/actions/runs/42104523666),
job `127515715480`: 359 required roots, zero skips, all 20 recovery repetitions,
the populated six-width browser regression and the 206-migration restore drill.
PR 38 merged as `451047a4a68e4da05a40656e42defd69e914bbd6`.

Final public candidate `a3ca806e984aaf0c24aad0e89befe9b31738d03a` passed
[test/install run 42104678564](https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/42104678564),
jobs `127516251098` and `127516251882`. Fresh installation and reinstall both
verified ordinary login, five readiness checks and identity preservation.
The exact export again passed both instance-secret scans and gitleaks.
Its 1874-entry FILES.json byte digest is
`955258a343a703c7919ea6d884695fecde6d83bf2ac913cbf47eff1f3340c891`.
[Atomic promotion 42106259828](https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/42106259828)
passed with expected old head `e2e6ba7`, preserved in
`ops/release/promote-verified-account-layout-20261105.yml`.

## Final live acceptance and remaining provider restrictions

The final guarded rollout completed on both independent instances on November 5
at 20:51 UTC. Both report public revision
`a3ca806e984aaf0c24aad0e89befe9b31738d03a` and schema 206. All 13 rollout and
acceptance stages exited successfully. Each instance passed 18 preservation
checks, five readiness checks, executable verification and verification of
25 served assets plus the configured panel HTML. No pending deployment remains.
All six application services are active with zero automatic restarts. The
production update included its normal intentional stop/start window; monitoring
recorded two nonready samples during that window. This was not a claim of
uninterrupted application readiness.

The protected backup vault on port 18443 retained the same process, with zero
restarts and the expected unauthenticated HTTP 401 response. Production retained
all 2401 profile ownership records and 10 Trojan ownership records unchanged.
Temporary audit administrators and sessions were removed. Updates used bounded
build cgroups with a 1 GiB hard memory limit, 768 MiB high limit and one CPU.
Final free space was about 22.4 GB; monitoring stayed above 20.0 GB free disk
and 1.05 GB available memory. The final health recheck passed at 20:54 UTC;
the temporary audit monitor then exited successfully.

All nine enabled account dashboards returned HTTP 200. The 12 diagnostic issues
match between account list and detail. Fresh Preflight checked all nine accounts;
four passed and five correctly exposed the provider failures below. Real Chrome
acceptance passed at widths 320, 390, 800, 801, 1024 and 1365 under the normal
production CSP, without injected CSS. Mobile and desktop screenshots were also
visually inspected; issues remain readable, with no document overflow or mobile
automatic zoom. Reload retained the diagnostics.

| Provider | Final live result | Remaining action or limit |
| --- | --- | --- |
| DigitalOcean | Three provider-locked accounts; Preflight exposes the failed identity/account checks | Resolve the lock with the provider; the API response does not establish a single cause |
| Vultr | One HTTP 401 authentication/API-access rejection | Check API credentials, permissions and source-IP access in the provider panel |
| Linode | Two successful identities; one authentication rejection | Repair the rejected account's API access |
| Kamatera | Successful identity and inventory | Three historical creates still have unknown outcomes |
| OVH | Successful identity and inventory | One historical create still has an unknown outcome |
| AWS | No saved account in either instance | Driver and fixture verification passed; live credentials were not tested |
| UpCloud | No saved account in either instance | Driver and fixture verification passed; live credentials were not tested |

The four unknown creates remain unchanged at one attempt each, without resource
or task receipts. They were not replayed. Authoritative provider evidence is
required before resolving them. Historical proxy-observation errors retain their
original timestamps independently of current proxy probe results.

Both Direct and Residential outputs returned HTTP 200, no-store and 80 valid URI
lines each. The system reported 20 active servers, 43 awaiting deletion/expiry,
four active build counters, no currently failed provisioning runs and 46 retained
historical diagnostic families. These counts do not assert a new successful paid
provisioning operation; no paid create was performed for this audit.

No new failed systemd units appeared. Three failed transient one-off jobs from
October 24–26 have no running process or triggers; their failure history was
retained rather than cleared. They are not current application-service failures.

The sanitized evidence is
[`ACCOUNT_DIAGNOSTICS_20261105-evidence.json`](../ops/release/ACCOUNT_DIAGNOSTICS_20261105-evidence.json).
Private receipts and genuine final browser captures are in
`/root/dob-account-layout-20261105` on the active controller. The earlier failed
layout acceptance and explicitly labeled browser-only preview remain in the
initial audit directory; neither is substituted for final installed acceptance.
