# Continuation — 2026-10-30

Base canonical source: b5e2d422c75414d24bf66391819abf3da6f98481.
Base published runtime: 68893142fff30c5567afdc7c3233ee9da0004721.
Live runtime remains 67f91d2886e0b2b6dec6754700066a35eb2fccae, schema 205.

## Current stages

- Prior three-country/browser feature: published and independently verified.
- Resume preflight: both instances ready; six services active, no pending deployment.
- Live outputs: eight raw routes and four viewers passed; no withheld entries.
- Historical Kamatera creates: three UNKNOWN, each attempt 1 without an action or resource receipt; no replay.
- Storage: 5,876,785,152 bytes free; production database 3,553,629,887 bytes; unchanged native reserve requires 11,197,760,573 bytes. Reproducible candidates total under 230 MB; deleting them cannot cover the deficit.
- Original host is online again, but its application services remain stopped and its quarantine remains authoritative.
- Live rollout/provider-site acceptance: blocked before changes by storage.
- Build Activity query improvement: implemented, independent tests pending.

## Read-only production evidence

The original page resolves operation diagnostics for the full 1575-row history
before returning 100 rows. The candidate materializes narrow activity metadata,
counts the full history and then resolves details only for the visible page.
Both versions returned identical rows in one database snapshot (EXCEPT ALL in
both directions); no settings or records were changed.

Two alternating EXPLAIN ANALYZE pairs measured original execution at 3231.620 and
1987.000 ms, versus candidate 146.110 and 32.799 ms. Operation-table scans fell
from 756 to 4 on that current dataset. These timings include cache/load effects
and are not an end-to-end panel latency guarantee. The separate installation
history endpoint still contributes load and remains unchanged.

A 1200-record isolated PostgreSQL regression bounds diagnostic table scans to
the visible 100-row page. Existing global total/terminal state/redaction checks
are now mandatory in the release gate. No migration or retention change is needed.
