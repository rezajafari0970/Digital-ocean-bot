# Config and Output recovery audit — 2026-10-18

## Candidate changes

Two of the eight profile-proof consumers prioritize due failed or missing evidence. Six preserve oldest-proof fairness; recovery consumers also fall back to routine work. All consumers retain the existing shared SQL-derived admission limit and per-panel ownership fence. A failed class cannot monopolize healthy refresh capacity.

Background route verification now tries panel ownership without consuming its native deadline in a lock wait. Busy panels receive a two-second version-fenced deferral. This preserves real failure history, leaves prior evidence unchanged, and records contention separately from completed or failed native attempts. Manual reconciliation retains its waiting and post-ownership recheck behavior.

Per-server Config details use the exact canonical Output publication predicate in the same repeatable-read snapshot. They show actual published client counts, the profile publication target and an explicit reason when native verification alone is insufficient. Scope, route freshness, class proof and snapshot freshness remain required; generic residual holds do not claim a more specific diagnosis than the evidence supports. Deletion status keeps precedence.

Strict HTTPS probes now emit only bounded attempt counts and allowlisted diagnostic codes. TLS certificate rejection, timeout, transport failure, empty body and HTTP status are distinguishable without exception text, bodies, headers or credentials. Diagnostics do not authorize publication. At most two attempts, existing deadlines, required positive TLS/HTTP/body evidence and permanent-error rejection are unchanged.

## Audit boundaries

Protocol, transport and security drivers remain typed, composable registry modules. These are code-registered plugins; this audit does not claim arbitrary runtime plugin loading. Direct and Residential identities, counts, limits, lifetimes, publication and Flow remain independent. Creation is idempotent, edits are revision fenced, ambiguous mutations require observation, and deletion holds port ownership until native absence is confirmed. Shared panel locking and durable leases protect native operations; cancellations do not manufacture profile proof failure.

Publication still requires current ownership and revision, confirmed clients, eligible scope and lifecycle, five-minute class/native proof, one-minute route proof, fresh routing membership, proxy health and a fifteen-second output snapshot. Existing client compatibility restrictions remain documented. Network/provider failure is not made impossible by recovery scheduling.

## Candidate validation

- Required disposable PostgreSQL, race and native regression gate: 41 passed, zero skips. Includes new contention/lease-version tests and expanded recovery-order, publication-hold and strict-TLS diagnostic cases.
- Mobile Chromium: 11 scenarios passed, including exact publication-hold messages, independent class controls, stale/hung reads, scoped deletion and DOM continuity.
- An earlier 40-test pass preceded the final publication-status/diagnostics changes; it is retained separately and is not substituted for the 41-test candidate pass.
- A four-minute baseline sampler found no gap in 121 atomic observations before deployment. Earlier baseline samples did have intermittent gaps; the zero-gap interval is not evidence that this candidate was already installed.

Full publisher gates, staged installation and post-release observations are pending at this checkpoint. No external OpenAI API calls were made.

## Live acceptance finding: provider retirement versus late proof

The first candidate was published as b42471e4a3c88bab1aedda362ae5bc91c2bf727c and installed on both controllers. The secondary passed all preservation checks. Primary readiness, exact binaries, configuration and secret hashes passed, but the native-identity acceptance correctly refused one inconsistent terminal status. Two servers had reached their pre-existing expiry/deletion lifecycle; all six profile remote IDs were retired and their credential references/hashes were unchanged. One late deadline receipt from the previous worker had overwritten `server_removed` with `runtime_retry` after confirmed provider deletion. A subsequent inventory found two retired rows with obsolete errors and no remaining managed provider resource for deleted servers.

The follow-up invalidates the durable work claim when provider-confirmed absence becomes terminal. Late batch failures, stage writes and outer retry receipts cannot overwrite RETIRED state. The ordinary reconciliation loop also normalizes old inconsistent terminal rows, retaining the original retirement timestamp and requiring the same DELETED provider state plus absence of any managed resource not marked deleted. No manual production SQL repair is used.

The mandatory lifecycle regression now reproduces both SSH failure and deadline completion after provider retirement, checks claim invalidation and unchanged credentials, and verifies automatic repair of a pre-existing obsolete status. All 41 required isolated database/race/native regressions passed again with zero skips. The previous mobile/Output result remains applicable because this follow-up changes no web assets. Follow-up publication and deployment are pending at this checkpoint.

## Final deployed acceptance

Final public release `16f3b682f638fba5021588fe9818b91db917ef66` (code source `44e8b1aa508e13efbf4077ad30691d63fa37c435`, schema187) is installed on both controllers. Secondary acceptance completed at 12:34:44 UTC and primary at 12:38:06 UTC. Each host passed exact live binary hashes, three active services, zero unexpected restarts, five readiness samples and policy/identity preservation. Later runtime checks remained healthy. The final publisher independently passed all 41 mandatory regressions with zero skips, full Go tests/vet, six Output viewer cases and the actual sandbox installation. The unchanged web assets passed all eleven mobile scenarios.

Primary's first follow-up install stopped before runtime shutdown because the second disk reserve check was 17 MiB short after compilation. Its failed log and intent were preserved; independently verified compiler/module caches were reclaimed and the unchanged reserve passed before the explicit retry. Four closed backup archives were losslessly compressed during this audit, saving 748,352,393 bytes; decompressed hashes and exact archive members/metadata matched. No backup was deleted and no reserve gate was bypassed.

Longitudinal comparison against the original release retained 1985 shared listener records with no unexpected credential/native identity changes. Fifteen changed remote IDs were independently matched to confirmed provider retirement; credential references and encrypted-secret hashes stayed unchanged. Both pre-existing obsolete terminal errors self-repaired to zero through ordinary workers, without a manual production database correction.

Main observation ran 12:38:28–12:44:33 UTC, 364.839 seconds and 25 samples. All readiness and Output requests succeeded. Twenty-three samples had complete coverage. Initial post-restart route refresh produced 462/480 links and recovered to 480/480 by the next sample. A later Residential HTTPS timeout after two bounded attempts withheld only that class while Direct remained valid. Automatic recovery started 11.163 seconds after the failed proof and committed fresh successful proof at 19.069 seconds. One Output sample was 479/480; the following samples were 480/480. Output latency was 34–51 ms, median40 ms. These results demonstrate observed automatic recovery, not the permanent elimination of network failures.

The 181 two-second diagnostic frames retained twelve incomplete observations, including the startup refresh, the class timeout and one newly enrolled server at the end of the interval. This last event increased the cohort from 240 to243 profile listeners and was not discarded. A separate six-sample follow-up verified automatic creation and proof for all three profiles on the new server: output reached486/486 on81 eligible servers. At the last follow-up at12:47:19 UTC, the eligible cohort had changed to80 servers and remained complete at480/480, with zero pending or blocked profiles. The final atomic check at12:47:38 UTC had240 eligible listeners and zero gaps. All follow-up readiness/Output requests passed.

All implementation, validation, publication and deployment stages in this audit are complete. Publication/TLS/ownership safeguards and independent class policies remain enforced. No percentage speedup, arbitrary hot-loaded plugin capability or permanently fault-free operation is claimed. External OpenAI API calls: zero. Machine-readable evidence is in `docs/development-jobs/config-output-recovery-acceptance-20261018.json`.


## Evening continuation: output completeness remains open

Fresh checks at 18:05 UTC and 18:14 UTC found both controllers still on `16f3b68`, schema187, with exact binary hashes, all three services active, no unexpected restarts and successful readiness. Environment, creation, Output, protection, legacy and unified policies are unchanged. Profile settings are unchanged after excluding only the expected worker scheduling timestamp. Longitudinal identity/credential preservation still passes for 1985 shared listeners; 117 native retirements are independently supported by provider-confirmed absence, and obsolete terminal errors remain zero.

This new observation does **not** repeat the earlier complete-output acceptance. At 18:05:59 UTC the current fleet had72 eligible servers and430/432 links. The five-minute sampler from18:06:22 to18:11:26 UTC collected21 successful Output/readiness responses, but zero fully complete samples. Published counts ranged408–425 while the target changed438–444 as the live fleet enrolled servers. Output response time was32–51ms, median37ms. All151 atomic diagnostic frames are retained, including incomplete ones.

Failures predominantly concern Residential class traffic. Two read-only native probes reproduced TLS EOF during connection establishment while the corresponding Direct requests succeeded. Other native probes on two initially affected servers returned valid TLS/HTTP200/body evidence for both classes; subsequent ordinary-worker database receipts confirmed all three profiles on both servers with two current class proofs. Recovery therefore works for observed instances, but recurring new failures prevented overall completeness.

An isolated diagnostic on one affected node tested four existing proxy outbounds directly, without changing the running configuration or publishing evidence. Two proxies timed out against both the observatory and Residential test destinations. One returned valid HTTPS from both; another reached the observatory but timed out against the Residential destination. This supports a current egress/connectivity bottleneck; it does not establish a new config-code defect or prove that any single threshold change would solve it. The existing3-second local observatory/selection settings and all publication safeguards were retained.

At18:14:10 UTC a separate follow-up catalog reported75 eligible servers, target450 and427 published links; the immediately subsequent Output request returned429 lines in48ms. These are separate reads while workers are progressing, not an atomic count mismatch. Two native proofs had just crossed the five-minute freshness limit. This snapshot is not labeled complete.

| Stage | Fresh status |
| --- | --- |
| Recover checkpoint and compare releases | Passed |
| Runtime readiness and exact installed binaries on both controllers | Passed |
| Configuration, credentials and native identity preservation | Passed |
| Ordinary-worker recovery and new-server reconciliation | Observed, with recurring new failures |
| Complete Config/Output acceptance | Not passed; Residential connectivity remains open |
| New code fix or deployment | None; no new software defect established |
| Record evidence | Completed in this continuation |

The prior41 mandatory,11 mobile and6 Output test results remain historical evidence for the unchanged release; they were not rerun or represented as fresh tests. No production SQL repair, policy relaxation, credentials change, native restart or new installation was performed. Next work should investigate the remaining Residential connectivity under the existing policy; complete output must be revalidated after that bottleneck is resolved. Machine-readable evidence: `docs/development-jobs/config-resume-evening-20261018.json`. External OpenAI API calls: zero.
