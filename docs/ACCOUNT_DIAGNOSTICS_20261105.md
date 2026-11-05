# Account and service audit — 2026-11-05

## Stages

| Stage | Status |
| --- | --- |
| Recover deployed revision and prior cleanup | Complete: both instances run public 9b2c75e, schema 206; prior cleanup completed |
| Read-only service/account audit | Complete: six application services and protected vault active, zero restarts; both instances pass five readiness checks |
| Reproduce missing account diagnostics | Complete: independent observation details absent from dashboard; generic Refresh message hides provider detail; failed Refresh does not reload persisted state |
| Shared backend/UI correction | Implemented; independent provider, observation and ambiguous-create issues visible in list and detail |
| Independent source/database/browser/race verification | Pending |
| Public installation, guarded rollout and live acceptance | Pending |

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
from the local workspace, so actual browser acceptance must run in independent CI.
Mandatory regressions cover seven-provider list/detail parity, secret boundaries,
ambiguous-create immutability, runtime failures, simultaneous preflight failures,
failed-response detail, navigation fencing and real mobile/desktop rendering.
