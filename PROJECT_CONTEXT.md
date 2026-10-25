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

## Current continuity entry — restarted audit 2026-10-25

Read the latest entries in PROJECT_STATE.md and HANDOFF.md and
docs/SYSTEM_AUDIT_RESTART_20261025.md with its inventory/acceptance JSON before
continuing. Current implementation bb6e4f9 is merged as a70937c8; public main/stable
and default installation are6c92d419. Production5dc2c256 remains disk-reserve
blocked. Original database/application/publisher quarantine remains in force.
The audit is incomplete (42 of1,218 baseline Go files fully read in this restart).

The API key is available privately on the original host. Its authoritative
schema2 ledger preserves one unknown request andUSD1.501011 reservation under
the unchangedUSD2/three-attempt cap. New paid submissions are blocked until
provider usage reconciliation. Never reset, duplicate or override this ledger.
The receipt/poll helper is tested; it has not completed a real model review in this
restart. No claims of permanent uptime, arbitrary unknown-bug auto-repair or
complete multi-host active/active safety are supported by the current evidence.
