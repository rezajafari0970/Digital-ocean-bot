# Output proof and protection receipt reliability — 2026-10-16

## Current boundary

Source e4e1e544 / public 124f51f is installed and verified on both hosts. Guardian 1/10/fleet rollout completed; observation scope all and serving capacity18 are active on both hosts. Output/proof acceptance passed. A separate network-identity scheduling defect discovered during acceptance remains open.

## Reproduced causes and fixes

1. Long unchanged native routing proofs exclusively blocked raw output refresh beyond its unchanged 15-second validity. Structural writers now take C then O; qualified no-op proofs retain C exclusive and downgrade O to shared; scoped output reads use O shared. Every structural writer, including raw purge/IPAM fences, participates. Feature defaults OFF until writer-complete rollout. A downgrade never restores mutation authority or falls back to mutation. Evidence writes use the owning PostgreSQL connection. Dead owners cannot publish through another pool connection.
2. Independent review found an ambiguous shared-lock acquisition could grant then return an error. A real PostgreSQL grant-then-raise regression leaked one lock and blocked a distinct writer on the baseline. Both acquisition/unlock uncertainty now invalidate ownership and discard the physical connection; the corrected race suite passes.
3. Planned guardian shutdown fabricated ERROR observations; a terminated owner could overwrite a successor receipt through a pooled connection. Both failures reproduced. Publication now uses the owning connection and a parent-cancellation-aware bounded context. Real operation failures remain errors and cannot advance verified evidence.
4. Serving proof consumers shared the two-slot maintenance class. Production had 88 current APPLIED rows but only 14 fresh proofs among 89 eligible panels; median proof age was 216 seconds while 88 raw snapshots were fresh. Dedicated bounded serving/retired admission starts before discovery, claims and the 45-second operation deadline. Conditional claim completion cannot clear or fail a replacement reservation. Authoritative scope/due/serving checks run after C/runtime ownership. Retirement cannot consume serving capacity.

## Capacity qualification and limitations

Startup capacity defaults to 2, is validated in 1..18, and is capped at 2 for combined/control roles. The split panels role may stage 2 -> 8 -> 18 after deployment gates. Parent 10, poll 2, maintenance 2, guardian lanes, bootstrap and PostgreSQL max40 are unchanged; API has a separate max24 pool.

The initial planning assumption of 30 long pins was corrected from source: one globally serialized client/cleanup/transport executor can hold an extra session. Conservative actual ledger is 31, leaving 9 short-query connections. A real PostgreSQL test held all 31 pins, exercised actual admission, configuration/observation locks, role/global executor locks, owning-connection writes, pooled reads, queued API writer and output observation. It completed 87 receipt writes, observed bounded pool waits and leaked no connections. This is modeled saturation of the audited ownership patterns, not an assertion that a production guardian was SSH-tested within that fixture.

Controlled-clock tests cover 89/90 panels, 9/10-second work, real 20-second due spacing, queueing beyond 45 seconds, cancellation and joined completion. A real-time PostgreSQL 90-serving + 1-retired fixture used actual production discovery, reservations, C/O and evidence transactions with a synthetic 10-second native phase: three cycles completed, maximum healthy receipt gap 50.166278 seconds. The synthetic fleet phase does not execute 90 native HTTP matrices. Full serial native matrix, unchanged proof, output refresh past 15 seconds, runtime drift, backend loss and writer starvation have separate real-HTTP/PostgreSQL regression coverage.

No validity window, native proof matrix, per-panel routeTest serialization, protection threshold or core binary is weakened. Growth beyond the qualified fleet or sustained slow proofs can still exceed capacity; telemetry reports backlog and stale output remains withheld.

## Deployment and acceptance

Normal publisher only. First update every API/control/panel writer on both hosts with observation OFF and low proof capacity; verify installed hashes and drain old writers. Qualify guardian artifact on one server, then ten, then fleet while preserving policy, existing Xray PIDs and listener structure. Enable output observation on a canary only after writer completeness. Ramp proof capacity separately, checking real completion histograms, output count/freshness, provider state, guardian receipts, SQL waits and pressure. Rollback capacity independently; disable and drain all observation readers before any old writer-code rollback.

Production can correctly report resource-pressure admission pauses and external proxy/provider failures. Such states are not erased to make dashboard counts zero. No permanent no-failure guarantee or full-project mastery claim is made.

Exact-source final full regression, vet and critical race/native/fault gates passed. Independent reviewer PASS; adversary found no correctness blocker, with sole G1 incomplete-test gate now satisfied by the retained results. Follow-up API request failed credit_balance_exhausted; no new model calls attempted. Evidence adjudication follows the project protocol; no missing code review is claimed as performed. API usage: {"requests": 30, "usage_available": 29, "failed_without_usage": 1, "input_tokens": 768667, "output_tokens": 444526, "total_tokens": 1213193, "estimated_usd": 30.4068445}. Publication and output runtime acceptance completed; network-identity follow-up remains open.


## Actual runtime acceptance

24 live DIRECT samples over 238 seconds found zero missing stable eligible servers and zero malformed/unwanted fields. Both complete checks of all11 existing API/global/account share surfaces passed; final ALL exported243 links across71 hosts and omitted zero of65 stable active hosts. DIRECT43 and RESIDENTIAL43 hosts reflect profile membership, not all-server counts. The output freshness window remains15s and route proof60s.

At capacity18, 89..91 of91 eligible route proofs remained current/fresh after warm-up; raw snapshots were fresh for91. Transient structural APPLYING states and three native HTTP header timeouts withheld affected class output until normal recovery. Pool40 used at most34 connections with zero pool waits. Old capacity2 had18 fresh proofs among93 eligible; capacity8 was insufficient for uniform60s coverage. No claim is made that failures or configuration transitions can never withhold output.

All91 guardians were observed on the new artifact after 1/10/fleet verification. Final policy/pin checks preserved186 primary pins and all account/environment/creation/output/protection policy fingerprints. Both hosts had all3 matching active service binaries and repeated readiness checks. Secondary has zero enabled accounts/panels/servers; all-scope observation/capacity18 are activated for future scope. A secondary verification-helper json/jsonb COALESCE type error was corrected by read-only verification without repeating activation.

Provider observation overdue remained0 throughout18 fleet samples. Recurrent PROXY_ERROR was traced further: network identity maintenance shares the single bulk-control slot with long lifecycle work, with a45s deadline starting before that queue. Healthy base proxy observations coexist with identity ages205..228s, beyond the unchanged180s economy guard. This separate scheduling problem remains OPEN; prior transient recovery is not evidence of its permanent resolution. Real CPU/PSI/conntrack pressure also legitimately keeps admission paused. The full project is not declared fault-free.
