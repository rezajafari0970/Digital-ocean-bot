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

## Current continuity entry — SSH, secrets and restore continuation 2026-10-25

Read the newest PROJECT_STATE.md/HANDOFF.md and
docs/SSH_IDENTITY_DURABILITY_20261025.md with its inventory/acceptance JSON.
Product4f2e595e merged via PR9; restore gate5801e0d1 merged via PR10 asbe4036cf.
Canonical branchcheckpoint/final-e2e-20260929. Public main/stable and default
installation run329d51a8; production5dc2c256 remains disk-reserve blocked.
Whole-project audit INCOMPLETE:138 full current files,105/1,218 baseline Go files.

Durable SSH reservation and scoped unknown reconciliation, malformed encrypted
record containment and retained-key reads are tested. Combined CI passes215
mandatory tests with zero required skips and an isolated backup/key restore,
corrupted archive rejection and interrupted transaction rollback. This is not
physical database repair, a production-backup restore or unconditional exactly-once
provider execution. Unknown intents without proof remain blocked for review.

Continue remaining worker/panel/provider coverage and resource/fencing/observability
work. Production update requires sufficient disk reserve plus build/backup margin,
fresh preservation evidence and the ordinary bounded updater. Do not bypass guards
or deploy the old retained8d838efa stage. Original services remain quarantined;
never run heavy tests or restart them.

The existing API key is private on the original host. Its authoritative schema2
ledger retains one unknown request andUSD1.501011 reservation underUSD2/three attempts.
No new paid generation was sent. Reconcile provider usage before another paid
review; never reset, duplicate or override the ledger. Permanent uptime, automatic
repair of arbitrary unknown damage and a fully reviewed project are not certified.
