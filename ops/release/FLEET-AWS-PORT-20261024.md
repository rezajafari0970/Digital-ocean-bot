# Fleet, AWS and panel ingress repair — 2026-10-24

Status: investigation and implementation in progress; NOT released.
Baseline private bb6b3bc4fcb19b1a6db4b7cb9512d4bec3bd52ed; live/public 6336c9e47f2907adc029024ea05fa433346b7e62.

## Evidence and scope

The 297-second supplied recording was reviewed at one frame per second (297 frames), including account creation, edits, outputs, fleet status and build details. Media contains credentials/configuration capabilities and must not be published or sent to external review APIs.

- Confirmed: account Edit throws `d is not defined` across providers. Optional selections and inherited modal state also need regression coverage.
- Confirmed: control recovery, lifecycle and maintenance share one bulk admission slot. A long installer serializes unrelated accounts despite a six-lane dispatcher. Any separation must preserve PostgreSQL headroom, checkpoint ownership and native mutation fences.
- Investigating: some READY panels have no output snapshots; Cloudflare WS profile verification remains pending on part of the fleet. Do not bypass publication proofs or inflate output counts.
- AWS first server reached READY during investigation and acquired eight confirmed profile clients. Four enabled profiles each request one client per DIRECT and RESIDENTIAL class: nominal eight configurations per fully verified server.
- AWS user spacing is 15–260 minutes; the recorded next build is a scheduling policy, not an installer failure. Preserve this policy.
- Public ingress is 8443 on the legacy gateway; application listens on 28080 on the healthy host. A database listen-port change alone would break the tunnel. Public port changes require a gateway adapter, reachability verification and rollback.
- Legacy machine remains quarantined for builds/databases. Its RDC connection is offline and existing SSH authorization failed. Public gateway management and the prior OpenAI credential cannot currently be reached.

## Stages

1. Full video/source/live evidence: complete initial pass.
2. Account edit/mobile regression: implementing.
3. Independent recovery and provisioning: investigating bounded admission changes.
4. Output coverage and self-repair: investigating.
5. Public ingress setting: architecture identified; gateway access blocked.
6. Focused tests, canonical transport gate, exact-SHA install CI: not started for this candidate.
7. Atomic release, backed-up deployment, live verification: not started.

## API ledger

Prior ledger: 3 requests, 6,809 tokens, estimated USD 0.05716284695.
This request: 0 external OpenAI API requests, 0 tokens, USD 0. Requested gpt-6-astra with highest documented reasoning (`max`) is blocked by absent credentials on the reachable host. Do not claim Astra review until an actual response and usage record exist. Current chat model cannot be changed through project tools.

## Implementation checkpoint before independent verification

Implemented, not yet released:
- Account Edit no longer references an undefined catalog variable. Optional plan/image/proxy selections remain empty and payloads deduplicate selections. Mobile Save state is reset and form submit supports Enter.
- Shared modal callbacks are restored after closing. Panel settings explicitly owns its submit handler.
- Recovery has a separate two-job class. Weighted control admission retains the existing provider/identity concurrency while bounding retained SQL to 17, total jobs to five, expensive jobs to two and the unchanged pool to 24 connections. Heartbeats expose recovery admission.
- Cloudflare allocation orders due servers by live endpoint coverage before schedule age. No live record is deleted to reclaim capacity. Missing endpoints distinguish fresh authoritative quota exhaustion from propagation.
- Public and application ports are separate in settings. A root-owned DOB_PUBLIC_PANEL_PORT declaration protects a forwarded deployment from breaking its internal tunnel; redirects retain the public port. Actual public-port editing remains blocked by absent gateway management access; this is not a completed ingress migration.

Verified: account editor browser passed for all six providers and four viewports before the later cross-dialog/settings tests were added. First canonical gate passed 161 required tests, zero skips. Final expanded browser/race/ownership gates are pending on an independent GitHub runner.

During the first gate the 4 GiB host experienced global OOM at 10:48:32 UTC (an unrelated supervisor Python process was killed). Both production worker role checks timed out and exited safely; systemd recovered both at 10:51:14 UTC. MemoryMax=2 GiB for the test unit was insufficient isolation from live load. Do not run further heavy native/race tests on this production host. Keep ownership fail-closed; do not hide the resource failure by removing its checks. Follow-up runtime and database verification is required.

Output evidence correction: eight confirmed profile clients per fully initialized server is not a fixed output-URI count. The Cloudflare profile may publish the same class client via multiple independently verified DNS endpoints. Observed healthy ten-server accounts published 49 configurations per class, not ten. The only enabled zone reported 199/200 authoritative DNS usage; uncovered new servers were waiting for capacity. Panels with no snapshots were past their server expiry and deliberately retained while replacement work finished; do not republish expired configurations.

Astra/API ledger remains unchanged: zero new calls, zero new tokens, USD 0. Historical total remains 3 calls / 6,809 tokens / estimated USD 0.05716284695.
