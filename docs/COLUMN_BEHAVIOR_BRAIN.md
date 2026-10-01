# Database Column Behavior Brain

Commit `5ec0fbc2c4c293d04841de677ad603373db25eaa`.

Live production columns indexed: **595**
Live production tables: **58**

- Columns with reader evidence: **512**
- Columns with writer evidence: **540**
- Columns with function mentions: **589**
- Columns with migration-origin evidence: **590**

Each entry combines live PostgreSQL schema metadata with commit-pinned migration/source evidence.

Reader/writer classification is conservative static SQL evidence. Dynamic SQL, repository indirection, generated queries, or aliases may require exact source inspection.

No application rows, DATABASE_URL, credentials, tokens, passwords, cookies, or secrets are stored.
