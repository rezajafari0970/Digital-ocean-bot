# Project Context
This repository is the durable source of truth for the DigitalOcean + Vultr automation project. Chat history is not the sole project record.

## Core rules
- Keep provider integrations modular and independent.
- DigitalOcean and Vultr capacity/account logic uses provider-specific authoritative data.
- Sanaei/x-ui live reads/writes use its API where implemented; Output must not depend on SSH reads.
- Output is live/fast rather than stale cached data.
- Proxy/account/provider modules remain independently testable.
- CAPTCHA/2FA challenges are completed interactively; authorized sessions may then be reused where appropriate.

## Continuity protocol
Before ending a development session: update PROJECT_STATE.md and HANDOFF.md with branch/HEAD, tests, deploy state, migrations, blockers, and exact next action; commit and push. Never store passwords, API tokens, private keys, cookies, or session secrets in these documents.

At a new chat/session: read PROJECT_CONTEXT.md, PROJECT_STATE.md, HANDOFF.md, then verify git status/HEAD and production service state before changing code.

## Current continuity entry — scheduler and worker continuation 2026-10-25

Active follow-up: supplied video, slow confirmed config deletion, intermittent database attention and protocol-valid TCP HTTP-header creation. Read newest handoff; these follow-up features are not complete.

Read PROJECT_STATE.md, HANDOFF.md and docs/SCHEDULER_DURABILITY_20261025.md plus
its inventory/acceptance JSON. Earlier SSH/secrets/restore evidence remains linked.
Canonical checkpoint/final-e2e-20260929; product 721b02b2 merged through PR11 as 9d24af68.
Public main/stable and default run dfe97084c84ad617609cafbff210b1e9fa31dc45; default schema 196.
Production 5dc2c256 schema 195 remains disk-reserve blocked. No production update attempted.

Whole-project audit INCOMPLETE: 213 full current files, 173/1,218 baseline Go files.
Scheduler ownership, partial reads and fail-closed preparation/SQL are fixed.
Final CI 219 required roots / 0 skips, 20 isolation repetitions and disposable restore
through 196 migrations passed. Independent fresh/repeated install and bounded
default update passed all 15 preservation checks. Complete reading of worker/
scheduler/resource modules is not an error-free or sustained-load certification.

Continue remaining application/panel/provider ownership and resource/observability
work. Production rollout requires sufficient disk reserve plus build/backup margin,
fresh preservation evidence and ordinary bounded update. Do not deploy the old
retained 8d838efa stage, bypass guards, delete incident archives or unrelated data.
Original services remain quarantined; no restart/heavy tests there.

The existing API key stays private on the original host. Its authoritative schema 2
ledger preserves one unknown request and USD 1.501011 reservation under USD 2/three attempts.
Zero new paid generation. Resolve original provider usage/result before another
paid review; never reset, duplicate or override the ledger. No unconditional
exactly-once, arbitrary physical repair, permanent uptime or complete audit claim.
