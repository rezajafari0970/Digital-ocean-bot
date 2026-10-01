# Volatile Audit / Status Policy

Continuity distinguishes **product-source drift** from expected operational evidence.

Product-source paths are `cmd/`, `internal/`, `migrations/`, `web/static/`, and `deploy/`. Uncommitted changes there block final continuity readiness.

`.audit/` is intentionally operational/verification evidence and may be untracked or modified. Generated status files such as `FRESH_CHAT_MASTERY_STATUS.json`, `KNOWLEDGE_DRIFT_STATUS.json`, and `PRODUCTION_REVISION_STATUS.json` may change when gates are rerun. Their dirtiness alone is not source drift.

Never delete `.audit` merely to make `git status` clean. Never claim production identity from a volatile status file without rerunning its verifier.
