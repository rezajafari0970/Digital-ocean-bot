# Independent Reality profiles — production continuation

Verified 2026-10-04. Runtime API and worker: `1ed067b5446366781df3a4a345f6b6063cca88a5`.
Canonical tree: `/root/projects/Digital-ocean-bot-canonical-e2e`; branch: `checkpoint/final-e2e-20260929`.
This checkpoint is documentation only; deployed binaries remain on the clean product commit above.
Always verify HEAD, origin, clean status, services, gates and current profile rows before further work.

## Product behavior

New selects Direct or Residential. Each type has its own card, edit revision, ports, owned config target, quota,
lifetime, device limit and durable creation rate. A target of one for each type is valid on the same port443.
Same-IP/port uses one physical listener; SNI, Reality keys and transport remain shared. Profile saves preserve them.
Mobile modal header/footer are fixed and fields scroll; Save stays reachable.

The former shared target/check-box validation is removed. Creation rate stays 1–100 credentials/sec; it does not
measure clients connected to a shared credential. Do not increase that guardrail to satisfy a concurrent-user target.

## Current user settings

During development the user changed the policy. Preserve these live edits and re-read before any mutation:

| Class | Port | Owned target | Quota | Lifetime | Device limit | Creation rate | Revision |
|---|---|---|---|---|---|---|---|
| Direct | 443 | 1 | Unlimited | None | Unlimited | 100/sec | 4 |
| Residential | 443 | 1 | Unlimited | 240 seconds | Unlimited | 100/sec | 5 |

Master enabled, ports [443], revision39 at the final DB capture. Earlier port1212 and no-expiry Residential
settings are obsolete and must not be restored from an old backup.

## Durable architecture

Migration148 creates `reality_config_profiles`, immutable ownership route classes and per-class rate buckets.
`global_config_policies` remains the shared listener/master cleanup switch. A profile edit cannot silently
resume a paused master. Cleanup guards and optimistic profile revisions remain active.
PLANNED identity and class are durable before POST; worker-only execution fresh-reconciles per item.
Unknown outcomes retry only missing identities; partial bulk success is never blanket success.
Quota/lifetime/HWID updates, shrink and expiry replacement operate on the selected owned class.

Manual/default clients are preserved and no longer counted toward a managed profile target. Physical capacity
snapshots add preserved manual clients to the target so observation remains truthful. Typed Output requires
ACTIVE matching ownership plus verified routing and requested URI port. Removed ports are withheld from typed
links. Aggregate legacy ALL behavior remains separate.

Settled scopes on removed ports close with `policy_port_removed` under the planner lock. Pending/running/failed
jobs prevent retirement. Budgets are not reset; manual resume does not rearm an obsolete port. Max-active-scope
guard remains64. No mass deletion of old inbounds/clients was performed by this policy migration.

## Verification

Focused PostgreSQL and race suites, full Go tests, clean detached full tests and real-Xray validation passed.
Both services are active with zero automatic restarts at capture. Full evidence and hashes are in the JSON
acceptance beside this file and `/root/backups/dob-independent-profiles-20261004`.

The isolated real canary created one additional identity per class, proved different quotas, deleted both via
the durable queue, and restored baseline identities. No mutation jobs outside the canary scope were created
during that scoped phase. Later current-policy proof validated Direct no-expiry and Residential240sec.

Fresh fleet API observation completed with32/32 requested scopes matching, each with one owned Direct, one owned
Residential and one preserved manual client on443. A first sample caught five normal expiry transitions; durable
deletion/replacement completed and the second read passed. This proves eventual replacement, not zero-gap service.
The user-configured four-minute lifetime continuously triggers that cycle.

The browser suite passed mobile layout, independent forms, validation, DNS catalog preservation, Output display
and share-class boundary checks. Recent billing layout, proxy separation and dashboard regression checks passed.
No claim is made that unrelated provider health is permanently healthy; counts change with fresh observations.
Expired READY-labelled droplets excluded from active scopes remain withheld rather than silently extending life.

## Runtime and continuation

Main client mutation gate is enabled, kill_switch=false, concurrency1, no panel/inbound scope; this restores
authorized automatic production operation. Legacy bulk gate remains closed. Do not close the main gate merely
because a development turn ends; fail-close it on a real mutation fault.
No manual utility may claim alongside the worker.

Source commits:
- ce2b89e: independent durable policies, UI and Output integration.
- c8b4c3e: preserve manual clients outside managed policy capacity.
- 1ed067b: retire removed-port scopes and filter typed Output.

Do not claim30000 simultaneous users were load-tested. Upstream UDP/QUIC support, provider-neutral capacity
selection, universal encrypted ad-domain classification, and previously blocked cloud resource deletion remain
separate work. Preserve those limitations and existing fail-closed residential routing.
