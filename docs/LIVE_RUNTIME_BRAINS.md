# Live Database + Runtime/Worker Brains

`LIVE_DB_BRAIN.json` snapshots production PostgreSQL **schema metadata only**: public tables/columns/default metadata, applied migration versions/checksums/timestamps and indexes. It deliberately stores no application rows, credentials or connection string.

`RUNTIME_WORKER_BRAIN.json` indexes worker/API/app concurrency and cadence evidence (goroutine starts, tickers, timers, sleeps/after) and captures live API/worker service state. Static evidence is navigation data; exact ownership/control flow remains source-authoritative.
