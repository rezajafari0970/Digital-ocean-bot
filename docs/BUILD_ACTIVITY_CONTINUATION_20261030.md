> Storage follow-up (2026-10-31 Tehran): [authorized cleanup](STORAGE_CLEANUP_20261031.md) raised observed free space to 31.68 GB. The final background-cleanup receipt awaits remote-tool quota availability; read it before continuing. The older disk figures below are historical.

# Continuation — 2026-10-30

Base canonical source: b5e2d422c75414d24bf66391819abf3da6f98481.
Base published runtime: 68893142fff30c5567afdc7c3233ee9da0004721.
Current published main/stable: 1081be7edc22bb0cd50ccfc93852aa1650f73406.
Live runtime remains 67f91d2886e0b2b6dec6754700066a35eb2fccae, schema 205.

## Current stages

- Prior three-country/browser feature: published and independently verified.
- Resume preflight: both instances ready; six services active, no pending deployment.
- Live outputs: eight raw routes and four viewers passed; no withheld entries.
- Historical Kamatera creates: three UNKNOWN, each attempt 1 without an action or resource receipt; no replay.
- Latest storage: 5,737,271,296 bytes free; production database 3,559,192,255 bytes; unchanged native reserve requires 11,214,447,677 bytes. Reproducible candidates total under 230 MB; deleting them cannot cover the deficit.
- Original host is online again, but its application services remain stopped and its quarantine remains authoritative.
- Live rollout/provider-site acceptance: blocked before changes by storage.
- Build Activity query improvement: implemented, verified and published; all 353 required roots and source/native/restore CI passed.
- Public main/stable promotion: complete, atomic run 41125845769; both refs re-read.

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

## Exact candidate and continuation

Source PR35: https://github.com/rezajafari0970/Digital-ocean-bot/pull/35
Tested source candidate: b500ef5f44fc0e61279dc6c4c7d81d5bb1b31d71.
Source CI 41124609102 passed all 353 required roots with zero skips, native browser
isolation, mobile flows, 20 lifecycle/recovery repetitions and schema-206 restore.
PR35 merged as f5dc4b4391d2d025ed3b123839049e3f05ff6e3b.
Public candidate: 1081be7edc22bb0cd50ccfc93852aa1650f73406.
Public CI 41124939606 passed source checks, fresh installation and repeat installation
on an independent Ubuntu 24.04 systemd guest, preserving identity and login.
Both live-secret scans and configured gitleaks 8.30.1 passed this exact export.
The immutable export SHA256 manifest was verified independently.
Atomic promotion 41125845769 passed; both public refs were re-read and match
1081be7edc22bb0cd50ccfc93852aa1650f73406. Publication does not mean live installation.

At 20:43:57 UTC, the production database had grown to 3,559,192,255 bytes;
required reserve was 11,214,447,677 bytes and free space 5,737,271,296 bytes.
Both live instances remain healthy at schema 205 / revision 67f91d2: six services,
zero restarts, ten readiness checks passed and no pending deployment.
No runtime update, migration or browser installation has started.

Resume only after enough disk is available for the current native reserve plus
build/browser/backup overhead. Roughly 8 GB additional free capacity covers the
present deficit with working room; always remeasure immediately before rollout.
Do not lower the guard or delete unrelated app data, retained backups or incident
records. The under-230-MB reproducible-cache inventory cannot close this deficit.
The old host is online but remains quarantined; use independent CI for heavy tests.

Prepared private helpers and a clean exact candidate checkout are under
`/root/dob-build-activity-20261030/` on the live host. They were syntax checked and
22 static assets were matched; the helpers have not been executed. Take fresh
separate instance baselines, run the bounded default update then the bounded
production update, and verify schema 206, executable/static hashes, settings,
credentials, identities and readiness. Finish with actual provider-site country,
route-failure and cookie-separation acceptance plus live output checks.
Do not replay any of the three UNKNOWN creates to test these features.
