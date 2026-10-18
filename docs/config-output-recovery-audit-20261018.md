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
