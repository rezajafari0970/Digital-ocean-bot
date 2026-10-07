# Fleet classified Output: confirmed diagnosis, not repaired

Observed 2026-10-07 04:20 UTC (07:50 Tehran). Read-only diagnosis requested by the user. No live gates, settings, clients, resources, binaries or static assets changed.

## Outcome and causal chain

The phone video shows blank Residential shared Output and one Direct line. Exact existing share links reproduce this on the server: Residential HTTP 200, zero bytes/links; Direct HTTP 200, one VLESS link. This is an empty eligible result, not just browser rendering failure.

At 2026-10-06 21:57:16 UTC (2026-10-07 01:27:16 Tehran), a temporary Sanaei runtime circuit for one panel propagated through the client executor and persistently closed the fleet execution gate. The ordinary 30-second panel retry became an indefinite global interruption. The main gate remains enabled=false, kill_switch=true with updated_at=2026-10-06T21:57:16.626276Z.

The surviving syslog line is definitive:
`2026/10/06 21:57:16 client mutation executor: sanaei runtime circuit open: retry in 30s; execution gate fail-closed`

Immediately preceding it, panel b18cbc7f-2228-4cd3-8757-53f7ec9748f0 reported native API login and CSRF transport timeouts. The circuit is per panel. The downstream gate is global. Job5079c33f-b4a6-4764-b610-e29718e0f9a0 on that panel is a strongly correlated attempted BULK_CREATE, but the executor log does not include a job ID. It later became OBSOLETE during server retirement; Reconcile overwrote its last_error. Do not present its current lifecycle message as the original trigger.

Residential profile lifetime is600 seconds. Replenishment ceased; the newest capacity snapshot is21:56:56Z and last successful BULK_CREATE21:57:09Z. No new mutation jobs were created in the latest20-minute window. Previously owned Residential clients expired, while preserved/default/manual clients do not satisfy the class profile ownership rule. Direct profile lifetime is0; one eligible owned link remains, still subject to server expiry.

## Publication evidence

At04:20:28Z the class funnel was:

| Class | Collected | Time/server eligible | Routing eligible | Profile ownership eligible | Actual shared links |
| --- | ---: | ---: | ---: | ---: | ---: |
| RESIDENTIAL |59|59|59|0|0|
| DIRECT |1|1|1|1|1|

The earlier04:12Z observation was61 Residential rows with the same zero ownership eligibility. Counts vary with normal fleet rotation; this does not change the filter diagnosis. Existing profile ports are443. At final collection11/12 enabled residential endpoints had a fresh healthy result; all59 candidate rows passed the actual route/health predicates. API and worker services are active.

The classified ownership filter is intentional and must remain. Publishing preserved clients as profile clients would misrepresent immutable policy limits. An ALL query is not equivalent to either class share and was the blind spot in the earlier UpCloud-only report.

## Source path

- internal/panels/sanaei/runtime_manager.go: repeated failures open a30-second per-panel circuit; Acquire returns ErrRuntimeCircuitOpen before acquiring a mutation runtime.
- internal/panels/clientops/executor.go: execute returns the Acquire error. RunOne durably schedules retry but still returns the original error.
- internal/panels/clientops/drain.go: returns that error to the worker.
- cmd/worker/main.go around273: any Drain error calls Journal.FailCloseGate.
- internal/panels/clientops/gate.go: transaction disables both main and old bulk gates; both live timestamps match.
- internal/panels/clientops/journal.go and internal/panels/usercapacity/lifecycle_admission.go: the closed gate prevents claims and new lifecycle admission.
- internal/adminapi/output_snapshot.go: fresh class routing and immutable ACTIVE profile ownership are required.
- web/static/output-live.js: empty successful text renders as a blank page, with no explanatory status.

## Separate findings and limits

The ambiguous next_retry_at SQL at the same timestamp belongs to internal/app/recovery.go. That query ignores its error; it is a separate defect, not the confirmed client-gate trigger. The retained worker syslog resolves this ambiguity even though journalctl no longer retains the incident window.

docs/UPCLOUD_OUTPUT_DIAGNOSIS_20261007.md remains valid for the separate UpCloud Desired5/capacity2 expired-server deadlock and incompatible residential trial ports. It does not explain the fleet-wide blank Residential share. Earlier inbound/global-policy disagreements predate the incident and must not be substituted for this trigger.

## Correction boundary for the next implementation

Handle specifically identified pre-mutation temporary panel unavailability with bounded durable per-panel/job deferral, without closing the entire fleet. Preserve failure closure for conflicts, terminal/invariant faults and uncertain outcomes; preserve fresh read-before-retry and exact immutable ownership. Do not indiscriminately swallow errors or auto-reopen every closed gate. Record durable closure reason/panel/job so logs are not the only evidence.

After reviewed tests and deployment, resume through the existing authenticated capacity domain action, reconcile admissible scopes without resetting historical failures/budgets, and verify fresh native client creation plus actual classified share bodies. Add fault coverage proving an unavailable panel does not stop healthy panels, and uncertainty/conflict still fails safely. Do not blindly retry the obsolete incident job. The UI should explain zero eligible links and paused automation without polluting plain subscription text.

Status: DIAGNOSED_NOT_FIXED. Runtime remains API/worker fc971058a555718cdd178cd226b038c65b79383f, static1384d16, schema160.
Evidence directory: /root/backups/dob-output-global-diagnosis-20261007. facts.json contains safe counts/settings, incident-worker.log has panel URL paths redacted; no share tokens or VLESS credentials are saved in the report.

Server OpenAI source/evidence review: CONFIRMED. Safe checkpoint evidence and review are in docs/OUTPUT_GLOBAL_DIAGNOSIS_EVIDENCE_20261007.json. This is diagnosis acceptance, not repair acceptance.
