# Kamatera regional create admission

Live continuation on 2026-10-28 found three ordinary Kamatera deployments at
PANEL_COMPLETE/READY in US-CH and US-NY2. Two creates in US-MI2 remained
unacknowledged after HTTP 5xx responses: no native task or server ID. Both durable
dispatch counts remained one; subsequent observations were not new POSTs.

Previously a different deployment could still send another create into that same
uncertain region. The driver now requires a durable regional claim. A short
PostgreSQL transaction serializes account/provider/region claims and checks for an
earlier attempted compute intent lacking both task and server identity. It commits
the new intent and attempt together, before dispatch, only when admission is clear.
No cloud call runs while this regional transaction is held.

A locally blocked, undispatched intent remains unprepared, so the workflow can
use its existing charged regional fallback. Its specific reason is
REGION_CREATE_AWAITING_RECONCILIATION. It does not mark account transport unhealthy
or prove an account quota limit. Other accounts and regions remain independent.
Once the prior task ID or server ID is durably recorded, new regional creates may
proceed. Accepted async tasks can therefore provision in parallel.

This guard never resolves the original uncertain operation, ages it into failure,
changes its reservation, resets its attempt count, or replays a billable POST.
Existing unknown operations continue the previous observation-only recovery.
Malformed legacy region evidence blocks new claims conservatively. Cancellation
is checked at claim time and the existing outbound mutation fence remains intact.

Regression coverage includes simultaneous regional claims, account/region
isolation, acknowledged task reopening, unchanged unknown outcomes after one day,
undispatched fallback, legacy missing-region evidence and account cancellation.
The mandatory isolated release gate includes all new tests; live acceptance and
publication status are recorded separately in the operational report.

Public pricing still omits regional Ubuntu mappings for US-MI2. The official
calculator and authenticated configuration were read without creating servers.
The native regional configuration supplies explicit zero Ubuntu licence prices.
The catalog now uses that exact regional evidence to fill missing public mappings,
while keeping all hardware/component prices public and verified. Failed or conflicting
evidence leaves the price unknown; no other region's price is substituted. The native
configuration also exposes account limit fields, but their enforcement semantics
have not been established sufficiently to replace the driver's unknown-quota
contract in this change.
