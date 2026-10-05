# Google Ads only — 2026-10-05

Latest explicit user policy supersedes the previous broad Ads list. Residential has exactly `geosite:google@ads` and the temporary exception `domain:browserleaks.com` (apex and subdomains). Removed `category-ads-all`, `category-ads`, and `facebook@ads`; do not reintroduce them on reconciliation or future server provisioning.

Product source and clean deployed API/Worker: `14fc055e2df887551f3e311991db9467d92363f6`. UI explains the exact scope. TCP/UDP support, managed direct IPv4 DNS, explicit Direct clients, fail-closed matched domains, private destination guards, profile controls and execution gates are unchanged. No migration, credentials or app-source change.

Focused race, installed-core fault, full Go, JS syntax and diff checks passed. AI OS Development Orchestrator and OpenAI API plan were used. Initial plan attempts lacked the API environment; after loading the existing environment the same ledger continued normally. No gate bypass.

Single-panel canary `024a75e3-893d-4642-81a3-30d4f8641302` passed 34 route checks before fleet rollout. API/Worker are active and readiness passes. Profile and gate snapshots are identical before/after.

Saved exact scope was independently read on 38/38 eligible serving panels at 11:48:53Z. The complete runtime matrix passed on 37/38 across the verification window. Panel `cf65a8f5-fce6-4b62-a5aa-51fccd518c35` remains runtime-unconfirmed: its initial route proof was unconfirmed and its bounded read-only retry could not obtain a fresh inbound snapshot. Its saved list has the correct two entries. Do not infer a successful runtime proof or manually mark health green.

Evidence: `/root/backups/dob-google-ads-only-20261005`. Original initial fleet failure and recheck failure are retained. The temporary category-scope diagnostic checks saved settings only; its success is not a dataplane or 34-probe runtime success.

Earlier upstream AUTH_REJECTED and Asho client timing remain separate unresolved observations, not retested or repaired here. No ad-display or maximum-speed claim. Continue from `docs/GOOGLE_ADS_ONLY_ACCEPTANCE_20261005.json`; prior DNS/dataplane evidence remains in `docs/ADS_UDP_DNS_ACCEPTANCE_20261005.json`.
