# Protocol health correction — 2026-10-26

Status: the confirmed code corrections are deployed and verified on both
instances. Proxy-path datagram variability remains unresolved. This record
does not claim that all service slowdowns or every possible code defect is fixed.
Last recorded live snapshot: 2026-10-26T06:10:52.238418+00:00 (UTC).

## Released source and exact verification

- PR 18 added native protocol evidence and native VLESS ID/Flow verification:
  merge c35d3ae26da090fbf6a16c9fe355eb85dbd0dfc1; 244 mandatory regressions.
- PR 19 corrected managed relay probing to respect its destination policy:
  tested source 175f85e845682a8417c65caac5944bb2de56f809,
  merge 163a202c6cbb777863c25f26e16ad6cefe162536; 246 mandatory regressions.
- PR 20 fixed the real TCP/UDP port-reservation collision:
  tested source b760678ad7919b1e8504174e5fdbcd41af960095,
  merge c385e4b7d4e204818db0420aee0a75dee1a5fbf4.
  Independent run 40273361859 passed 247 mandatory roots with zero skips,
  including disposable PostgreSQL, pinned native transport, race checks,
  browser checks and a real backup/restore/transaction-interruption drill.
- Public release 7d2981ccacdd90da8306bbfcb54d1294f33afdfb passed exact-source checks and fresh/repeated
  installation in run 40274047231, with login and identity preservation.
  Run 40274292251 verified source/install results and promoted main/stable
  atomically with expected-head leases.
- Export scans compared 40,728 production secret values and 15 default-instance
  values without logging them. Pinned/checksummed Gitleaks also found no leaks.

## Implemented behavior

The module probes native transport settings in an ephemeral loopback-only Xray
process. Credentials travel through authenticated SSH/stdin and are not printed.
External SOCKS requires fresh valid DNS and two QUIC version-negotiation replies.
Managed TLS/XUDP probes two permitted-domain QUIC replies, preserves the donor
deny policy and does not claim DNS capability. No ad HTTP requests are sent.

Evidence binds the origin, deployment generation, driver, complete outbound
credentials/TLS/XUDP settings and probe program. Changes invalidate old evidence.
Four workers use fenced, expiring claims, a 75-second operation budget and a
65-second remote process-group timeout. Requested refresh is two minutes;
observed first-to-second fleet cycles had median
358.1 seconds under this workload.
Evidence expires after ten minutes. A transport failure cannot renew old proof.

The established configuration owner applies changes and verifies the native
plan. TCP selection remains independent. Protected UDP has no direct fallback:
an empty verified pool denies it. The scope is running as all on both panel
workers and includes newly eligible nodes.

The socket fix reserves the same loopback port for TCP and UDP with at most 32
attempts. Only EADDRINUSE is retried; failed/partial allocations close their
sockets. A mandatory real-socket regression covers occupied UDP, exhaustion,
held reservations, unrelated listener preservation and descriptor cleanup.

Native VLESS verification compares ID and Flow as well as email. Hot-updated
users can differ from the startup JSON, so that difference alone is not proof
of a broken live account. The OpenAI CLI argument bug is also fixed; its
existing unknown-cost and budget guards are retained.

## Deployment and preservation

| Stage | Result | Recorded evidence |
|---|---|---|
| Default final update | Passed 2026-10-26T05:31:49.807904+00:00 | 16 preservation checks; three services ready, exact artifact hashes, zero restarts |
| Production final update | Passed 2026-10-26T05:41:10.440716+00:00 | 16 preservation checks; 6,897 existing profile-ownership records and 77 Trojan records unchanged |
| Database | Schema 200 retained | Identity and integrity verified; sealed backup source 20261026T054019.237062Z |
| Scope | All on both actual processes | UDP enforcement and selective policy read from running worker environments |
| Canary 79.137.10.153 | Passed 2026-10-26T05:48:26.883784+00:00 | 40/40 QUIC VN and 8/8 neutral HTTPS requests across four actual VLESS paths; production file unchanged |
| Native fleet | 99/99 distinct nodes passed in two cohorts | 396 native VLESS identities/Flows matched |
| Automatic observations | Execution/coverage passed at 2026-10-26T05:59:27.497940+00:00 | 98/98 observed, zero execution errors, all routing APPLIED, 96 surviving initial nodes observed twice |
| Latest network snapshot | Variability remains | 99/99 fresh observations; 96 nodes with a positive UDP candidate |
| OpenAI generation | Blocked; no new paid request | Existing unknown request and conservative reservation retained |

Earlier production attempts for release 13570e5 stopped twice at the free-space
guard before replacing the runtime. Lossless, checksum-verified archival
recovered space and the third attempt succeeded. Those failures and the initial
negative canary observation remain recorded. The socket follow-up succeeded on
its first deployment attempt for each instance. Reproducible build/APT caches
were cleared after compilation to preserve space; the capacity guard was not
weakened. Runtime/configuration/backup evidence was retained.

## Remaining network evidence

The initial server DNS sample passed 96/96 requests (median ranges 1–11 ms,
maximum 26 ms), with about 93.8% CPU idle. Short 4 MiB transfers measured
119–164 Mbps direct and 3.19–7.90 Mbps through proxies, including connection
setup; these are not maximum-capacity measurements. Eleven external SOCKS
endpoints rejected UDP_ASSOCIATE; only P12 demonstrated UDP.

Initial post-release canary QUIC passed 40/40 and HTTPS passed 8/8. Longer fleet
observation nevertheless showed variable datagram results. At 05:59 UTC only
52 of 98 eligible nodes had a fully positive generic UDP result; this is not
hidden by the successful code/coverage checks.

A bounded differential at 2026-10-26T06:01:08.027064+00:00 reproduced this outside the native
probe: direct controls passed 12/12; raw SOCKS DNS passed 4/6, with two timeouts,
while raw SOCKS QUIC passed 6/6. Two native two-second trials passed, but a
five-second trial still timed out on DNS. This supports a proxy-path problem,
not simply a too-short two-second deadline; the exact upstream cause is not
established. It does not justify blindly changing DNS, MTU, BBR or all timeouts.

The conservative generic UDP gate requires both DNS and QUIC, so partial
capability is not admitted as fully verified UDP. TCP remains available through
its existing selection. A stable additional UDP-capable path is needed for
redundancy; none of the other eleven measured endpoints provides it. Client/ISP
latency and full HTTP3/video behavior still require client-side evidence.

One transient probe_unavailable state was seen at 06:08 UTC and had cleared
automatically by the 06:10 diagnostic. The port fix does not claim to eliminate
all future SSH, process or network failures.

## OpenAI status and traceability

Read-only model metadata previously confirmed gpt-6-astra. The authoritative
schema-2 ledger still contains one unknown TimeoutError request without a
response ID and a USD 1.501011 conservative reservation under the USD 2 cap.
Actual usage/cost remains unknown. No new generation, ledger reset, duplicated
ledger or guard bypass was performed.

Private operational records are under /root/dob-protocol-health-20261026:
completion-evidence.json, stages-current.json, both socket rollout directories,
socket-fleet-coverage.json, native-audit summaries, canary acceptance and the
provider differential. Secrets are excluded from the user-facing report.
