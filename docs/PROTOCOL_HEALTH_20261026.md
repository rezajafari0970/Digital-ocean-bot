# Protocol health correction — 2026-10-26

Status: implementation and independent verification in progress. This document
does not assert a completed rollout.

## Confirmed baseline

The authorized test node is 79.137.10.153. Controller-side API model metadata
confirms access to gpt-6-astra. The authoritative OpenAI ledger retains one
unknown synchronous request without a response ID and a USD 1.501011 reservation
under its USD 2 cap. No new generation has been submitted; model lookup was
read-only. The budget and unknown-outcome guards remain intact.

Previous node measurements found eleven external SOCKS endpoints rejecting UDP
and one passing datagram tests. TCP remains usable on those eleven endpoints.
An isolated warm A/B routed residential QUIC successfully 6/20 times with the
old UDP pool and 20/20 with the measured UDP candidate only. These are QUIC
version-negotiation samples, not application throughput or a reliability SLA.
Measured residential download rates and latency remain provider constraints;
server DNS, CPU and RAM did not show a bottleneck during those measurements.

## Changes

- Native transport driver probe: one loopback-only, ephemeral Xray process,
  credentials in encrypted stdin, pinned core hashes, no production file edits.
  The same SOCKS, TLS pin and XUDP outbound settings are exercised, including
  managed relays. External SOCKS protocol labels do not prove UDP support.
- External SOCKS requires a valid fresh DNS response and two QUIC version-negotiation replies. Managed TLS/XUDP requires two permitted-domain QUIC replies; its intentionally denied DNS path is not tested. Responses bind the transaction, connection IDs, target and port.
  External SOCKS uses fixed 1.1.1.1 endpoints. Managed relays use googleads.g.doubleclick.net for version negotiation only; no ad HTTP request is sent.
- Per-panel durable evidence binds origin, deployment generation and complete
  outbound fingerprint. Program and credential changes invalidate evidence.
  Refresh interval is two minutes; routing rechecks reject evidence older
  than ten minutes or from the future. A transport/SSH failure cannot refresh
  old evidence or certify a negative result.
- Four bounded workers use durable, expiring claims. Replacement owners fence
  late completion; complete evidence and rescheduling commit atomically.
  All configuration writes still use the existing owner and native proof path.
- TCP candidates and their existing health selection are retained. Only
  measured UDP candidates enter UDP balancers. An empty UDP pool denies its
  protected traffic and cannot silently route it directly.
- Native VLESS verification checks account ID and Flow as well as email.
  The panel can hot-update users without rewriting its startup JSON, so that
  file alone is not evidence of a broken live Flow or restart behavior.
- OpenAI submit CLI no longer references an undefined argument. Tests ensure
  the existing unresolved-cost guard remains reachable and is not bypassed.

## Rollout scope

DOB_UDP_CAPABILITY_PANELS accepts all, none, or validated panel UUIDs.
Default is disabled to permit a measured canary before fleet activation.
Schema migrations 199 and 200 retain prior migration history. Keep the previous executable and
backups; do not remove existing release, ownership, disk or secret-scan gates.
The native VLESS proof applies generally.

## Recorded stages

| Stage | Status | Evidence |
|---|---|---|
| Source baseline | Confirmed | 4f12dff237e56eea7c71110b24c5885e090a0c88, clean isolated worktree |
| Strongest configured API model | Available; generation blocked | Read-only gpt-6-astra model lookup and unchanged authoritative ledger |
| Native probe canary | Passed | New probe rejected P01–P11 and admitted P12; DNS + two QUIC replies; production configuration unchanged |
| Response validation | Passed | Malformed/replayed/misdirected DNS and QUIC response checks |
| OpenAI regression | Passed | 24 tests; CLI submission reaches existing budget guard |
| Isolated database/native/full CI | Pending | Must pass exact candidate SHA with no required skips |
| Production canary | Pending | No project or target configuration has yet been deployed |
| Fleet rollout | Pending | Depends on independent CI, release gates and canary |

Controller-to-server and phone-to-server paths are different. Client/ISP
congestion, full HTTP/3/video behavior and residential provider capacity cannot
be declared fixed by these server-side probes.

## Pre-fleet managed-relay correction

Before fleet activation, review of configureRelayInlet confirmed that generic DNS/IP probes violate the existing donor category policy. A second native-driver probe now tests permitted-domain QUIC through TLS/XUDP and retains the deny policy. A native regression exercises both the successful permitted probe and the correctly rejected generic probe against the same donor. Driver and donor-plan changes invalidate evidence.

First source PR 18 and public release d43ef52 passed 244 mandatory checks, fresh/repeated installation and atomic promotion. The independent default instance was updated with all 15 preservation checks passing. UDP enforcement remained disabled, production remained on 2755f99, and fleet activation is held for verification of this correction.
