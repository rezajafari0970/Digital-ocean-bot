# Evidence Gap Closure

Commit `be30070a041aee16100278b60538fb14067c239c`. This file distinguishes **closed/explained** gaps from **confirmed remaining** gaps. Missing evidence is never converted into coverage by assumption.

## Findings
- `residential/tests` — **confirmed-gap**: No direct Residential test evidence exists in current repository; keep as knowledge/product verification gap, do not mark covered.
- `database/migration-origin/provider_catalog_cache.catalog` — **explained**: Origin cannot be assigned to a CREATE migration from current parser evidence.
- `database/migration-origin/provider_catalog_cache.refreshed_at` — **explained**: Origin cannot be assigned to a CREATE migration from current parser evidence.
- `database/migration-origin/schema_migrations.*` — **explained**: These columns are bootstrap-owned by migration runner, not a numbered migration.
- `database/code-mentions/security_profile_columns` — **confirmed-indirect**: Do not fabricate reader/writer ownership; access may be indirect/unused.
- `database/code-mentions/proxies.expected_exit_ip` — **confirmed-indirect**: Keep as direct-evidence gap until source proves access.

## Direct historical Git evidence
- `sanaei_panel` — 6 directly selected historical commits
- `reality` — 5 directly selected historical commits
- `admin_api` — 3 directly selected historical commits
- `frontend` — 6 directly selected historical commits
- `residential` — 0 directly selected historical commits
