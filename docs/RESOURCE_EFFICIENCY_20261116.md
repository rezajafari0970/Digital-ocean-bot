# Resource efficiency — 2026-11-16

Targeted work on Digital-ocean-bot, independent of other projects. Baseline source 40c126f70d4da751dc14529437ffee21984449cc; installed public revision 019def54477ceaae3e29bc7c7b7403db5258e1c9. No new runtime has been deployed at this source checkpoint.

## Measured baseline

Two CPU / 3910 MiB shared host, 66 percent filesystem used and about 2287 MiB memory available at first observation. Twenty-second production DB sample: 18736 committed transactions and 264 new backend sessions over 20.53 seconds (includes diagnostic sessions). Production PostgreSQL used about 61.8 percent of one core; default PostgreSQL about 20.6 percent. Application samples: production API 7.73 percent, panels 6.19 percent, control 0.66 percent. These are short samples, not long-term guarantees.

Production database ~1.58 GB; panel_inventory_syncs ~757 MB, provider_snapshots ~283 MB. Current two application runtime directories total ~146 MB. Historical production rollbacks and a 369541120-byte project-specific Go build cache are separate from active data. Other projects were inspected only for attribution.

## Change

Independent discovery controls coalesce empty profile queue polling across sibling consumers. A committed claim opens full existing parallel admission before remote execution. Create, delete, routine verification and recovery verification use separate gates. Existing one-second cadence, work budgets, SQL ownership, failures, cancellation, publication freshness and provider routing remain unchanged. No SQL-pool or advisory-lock safety guard is weakened.

The production rollback archive driver accepts only exact dated project runtime directories, retains three newest and all versions younger than seven days, checks running process references and shares installer locks. It verifies binary identity and every archive member/hash/permission before replacing an inactive copy with a durable archive pointer. Interrupted removal resumes only against a verified archive and unchanged remaining files. Archives preserve all versions; no database, settings, user data, incident evidence or other project's tree is eligible. The optional independent hourly timer processes at most two copies at low CPU/IO priority, with 128 MiB memory and 25 percent of one CPU. Failed checks preserve data and remain visible.

## Verification state

Local focused Go race tests passed for discovery and existing worker profile admission. Five archival tests passed (roundtrip/compression, ownership and active-use refusal, three-version retention, interruption recovery, corruption refusal). Mandatory regression gate adds five discovery roots and a real PostgreSQL committed-claim / panic-cleanup test. Full independent CI and live acceptance remain required before product deployment.

OpenAI gpt-6-astra review was attempted with high reasoning, 4096 output-token cap and a conservative 0.236559 USD reservation inside a one-dollar/two-request session ceiling, using the existing authorized durable ledger. Provider response failed credit_balance_exhausted with no usage or review text. No repeat generation or legacy ledger reset. Reservation is not an asserted charge.

Private evidence: /root/dob-resource-audit-20261116. Keep original-host quarantine and unknown cloud-create receipts. Exact next: isolated source and install gates, then bounded deployment and equivalent before/after resource and functional measurements. Do not claim achieved live CPU/RAM reductions before acceptance.
