## Integrity timer recurrence fixed and deployed — 2026-10-23T18:07:29.373262+03:30

Public 380209b / product source dd1f8fc is installed on both hosts. A real primary defect (active/elapsed integrity timer with no future trigger after reboot) reproduced in a disposable systemd fixture. UTC six-hour calendar scheduling with persisted catch-up fixes the missing deadline; two real-systemd tests and installed-timer scheduling assertions now gate normal releases. All 143 mandatory regressions/no skips, full checks, exact LXC smoke, both GitHub CI runs and both-host rollout passed. All six running hashes, readiness and unchanged settings/credentials are verified; all 4,484 common profile and 77 Trojan identities unchanged. No new database checksum event: primary acknowledged watermark one, secondary zero. Both health timers actually wait for finite executions. Publication/backup/evidence timers are active on primary. No saved AWS account exists; real onboarding and physical corruption attribution remain unverified. Direct model API usage zero. Continue from docs/integrity-timer-20261023.md and its development job; evidence /root/backups/dob-integrity-timer-20261023. Prior entries are historical.

## 2026-10-23 afternoon AWS/controller/database acceptance

Public f548fb6115e896e8cca07fd2b958df7daccacdcf from product source 024f24bd3927c410b3e50d0443d107cd15e13ca8 is installed on both hosts. DescribeRouteTables now uses its documented 100-item limit; unknown pagination actions fail before HTTP. Both controllers now use the same kernel-memory guard as provisioned nodes, with installation preflight and independent five-minute monitoring; serial boot proof confirms kho=off/CMA=0. All 143 mandatory regressions, full checks, exact LXC install and both GitHub CI runs pass. Six runtime executable hashes, both readiness checks, settings/credentials and ownership preservation pass. Actual mobile accounts/AWS form render without overflow or JavaScript errors.

A post-boot full check found one Cloudflare heap page corrupted despite the earlier successful physical/logical validation. The entire stopped cluster was preserved; a verified backup page with identical header/LSN was tested in an isolated copy, then restored offline. Only 16 damaged bytes differ in the original relation, full checksums/amcheck and 15,264-record digest comparisons pass, and direct disk readback plus a new encrypted off-host backup are confirmed. No row deletion, full DB rollback, checksum bypass or counter reset. Physical damage cause is still unproven; do not claim all infrastructure risk is fixed. The three exact video SSH/install runs all completed naturally with active panels. No saved AWS account exists, so real onboarding/EC2 acceptance remains unclaimed. API usage 0 requests / 0 tokens / USD 0. Details: docs/video-afternoon-20261023.md and docs/development-jobs/video-afternoon-20261023.json; operational evidence under /root/backups/dob-video-20261023-afternoon.

## AWS diagnostic and provider-switch repair deployed; live credential retry pending — 2026-10-23T05:21:47.965211+00:00

Public cfdedbf / private product source 26324e9 is installed on both hosts. AWS errors now carry allowlisted native code, operation, stage, permission guidance and a local trace without credential/raw-response leakage; signature/token/account-lock classifications and non-AWS response handling are repaired. Leaving OVH restores the shared validation button label. All 138 mandatory regressions, full release checks, final mobile browser, exact installer, both GitHub CI runs and both-host six-service/hash/readiness/preservation checks passed. No AWS account is saved; the video's original rejection cause and live onboarding/create remain unverified until one fresh user preview. Preserve current operator policies. A retained intermittent Trojan queue-test timeout passed on full rerun; a quarantined derived Go cache mismatch has no established physical cause. Direct model API usage zero. Continue from docs/aws-validation-20261023.md and docs/development-jobs/aws-validation-20261023.json; evidence on both hosts under /root/backups/dob-aws-validation-20261023. Earlier entries are historical.

## Video defects repaired and deployed with live acceptance — 2026-10-23T04:41:54.644591+00:00

Public `d91040b` / private product source `c3f4bc7` is installed on both hosts. Fixed verified LABEL/UUID root alias handling in the OVH Ubuntu kernel guard, transactional Save receipt, read-only pending VM ownership and JSON-null inventory regions. All 134 mandatory regressions, full publisher, isolated/mobile browser, exact installer, both CI workflows and both-host six-service/hash/readiness checks pass. Preserve OVH target 10, lifetime 993–1006 minutes, spacing 39–96 minutes, selected regions/plans/OS and apply-to-existing OFF. At 08:10:15 Tehran: OVH one READY and three WAITING_INSTALLER with no current provision error; account Direct/Residential five links each; four Reality links native-matched and actual HTTPS passed; all 80 routing panels verified. Desired target is still filling through ordinary automation. Residential proxies fluctuate (9/12 healthy in final sample), and physical cause of two repaired/quarantined cache artifacts is unestablished. No blanket uptime or all-protocol/fleet connection claim. No OpenAI API usage in this continuation. Continue from `docs/video-repair-20261023.md` and `docs/development-jobs/video-repair-20261023.json`; evidence on both hosts under `/root/backups/dob-video-repair-20261023`. Earlier entries are historical.

## OVH full catalog and pricing deployed and live verified — 2026-10-23T03:37:31.027949+00:00

Public `98ac1d7` / product source `738b282` is installed on both hosts. Live: 204 plans, 82 supported/in-stock, 203 officially priced, H200 unknown; FR/EUR reference rates exclude network/tax and use 730-hour estimates. Three reproduced stale-catalog regressions repaired; 130 mandatory database/race tests, full release, both GitHub workflows and production mobile acceptance passed. Catalog fetch time/version and health observation are independent. No unexplained ownership changes. One corrupt test-container ARM64 cache object repaired with retained evidence; physical hardware/storage cause remains unresolved. This continuation model API usage zero. See `docs/ovh-catalog-pricing-20261023.md` and its acceptance JSON. Older generated indexes describe historical source; use this product revision for the new catalog path.

## XHTTP output storage repaired and verified — 2026-10-15T16:54:59.420628+00:00
Supersedes the earlier statement that ordinary fleet XHTTP output was not yet observed. The production defect was the full URI primary-key index: long XHTTP links exceeded PostgreSQL2704-byte btree keys, and ignored persistence errors left Output empty for these listeners.
Product fixaaf0ccbf7464d33e744a4c67d94e45acb8411941; public main/stable and BOTH installed runtimes56c0b8e8ad2b7af19cfec862acfef9c68d11b56e; schema179. New digest index preserves complete URIs and prior timestamps, guards full-string equality, rolls back failed snapshots, and logs credential-free stage/SQLSTATE failures.
Actual4096-byte PostgreSQL regression FAIL before/PASS after; full isolated PostgreSQL/browser suite, focused output/account/profile race, exporter/secret gates and exact Ubuntu24 installation PASS. Both GitHub workflows PASS. Secondary then primary normal updates preserve accounts/environment;3active hash-matching services and5/5readiness each.
At2026-10-15T16:53:40.082270+00:00 existing real shares publish XHTTP ALL36/Direct7/Residential21 across8ordinary listener paths; all inspected XHTTP paths4096, Host apart.com, stream-up, exact Extra and empty Flow. Snapshot counts vary with creation/routing freshness. No output persistence errors observed after restart. Final fleet39=37active+1degraded+1pending deletion; no all-fleet health guarantee.
Existing TCP remains TCP. FinalMask saved disabled and WebView incompatibility remain explicit. No new phone/SDK test, provider/IP mutation or unrelated lifecycle work.0new model API requests/tokens in this repair. Evidence .local/xhttp-evidence/output-fix-* and /root/backups/dob-xhttp-output-fix-20261015/live-output.json.

## XHTTP creation defaults deployed and enabled — 2026-10-15T16:23:43.399698+00:00
This entry supersedes the publication-pending XHTTP entry. Product source d671e19 produced public main/stable 89292e72b9feb2addebaf1bd4dd39e12b09d216c. Both control-plane hosts run that exact revision and schema178. All three running binaries match manifests; readiness5/5, ordinary login/API, new asset, account identity and environment preservation passed. GitHub main and stable checks passed. Creation policy revision2 selects XHTTP Reality stream-up, apart.com Host and4096-byte pinned paths on both hosts.
Existing TCP listeners and pre-existing pins remain TCP. Creating another user on an existing TCP listener still produces TCP. A normal newly-created fleet XHTTP listener was not yet observed at activation; do not claim all servers converted or end-to-end acceptance across every provider.
All36reviewed product/test hashes match PASS; full isolated PostgreSQL/browser suite, final affected-package race, browser-r3, vet/diff passed. Native X-UI create/retry/restart/readback and phone16/16allowed requests passed; temporary canary cleaned up, main panel active. Operator identification was unavailable on this repeat, and ad tests are static assets rather than SDK display.
Exact FinalMask preset remains saved but DISABLED: tested Xray26.9.9 rejects numeric brutalUp and UDP exp. WebView disabled; Flow empty. These requests are compatibility-limited, not completed activation.
Final fleet at16:22:12Z:39tracked=29active+5inactive(PROVIDER_UNAVAILABLE)+5pending deletion,0degraded,0broken. No provider recovery/IP allocation or unrelated rotation work performed.
OpenAI task:7responses,61103input+13941output=75044tokens; this continuation0new inference requests. Five GETs retrieved prior usage receipts. Actual billed amount unknown.
Resume from docs/XHTTP_PROFILES_20261015.md and the same development job. Feature publication/rollout is complete with the explicit compatibility limits above. Do not restart paused additional-IP work without user direction.

## XHTTP profiles verified; publication and rollout pending — 2026-10-15T16:12:24.814032+00:00
Current task is XHTTP creation defaults. Additional-IP sender/rotation work remains paused.
Source lives in /root/projects/dob-xhttp-20261015, branch feat/xhttp-profiles-20261015.
New physical listeners can use XHTTP Reality stream-up, Host apart.com, stable unique 256–4096-byte paths and the exact requested six-field client Extra. Existing listeners retain TCP and their credentials. SNI follows the existing scanner policy. Shared inbound clients share its path.
The exact requested FinalMask remains stored and DISABLED because tested core26.9.9 rejects numeric brutalUp and UDP exp. Browser WebView remains disabled and Flow empty for this profile. Do not claim these requested incompatible features work.
The source review PASS matches all36 changed product/test files. Full isolated PostgreSQL/browser project suite, final affected-package race, transport browser tests, vet and diff checks pass. Native X-UI create/retry/restart/readback verified4096-byte path; phone16/16 allowed probes/static ad requests passed before/after restart; temporary canary removed. A previous cloudflare trace failure was expected Residential allowlist behavior, not failed XHTTP transport. Phone operator identification failed on this repeat; no fresh MCI or SDK-ad-display claim.
Both live hosts remain public473e991/schema176 at the predeployment check,3active units and readiness5/5. Publication through existing private exporter and exact-candidate gates, then secondary/primary updates and authenticated default activation are next.
OpenAI task7responses,61103input+13941output=75044tokens. This continuation0new inference requests;5GETs recovered prior receipts. Actual billing unknown.
Evidence: docs/XHTTP_PROFILES_20261015.md and docs/development-jobs/xhttp-profiles-20261015.json.

# Verified additional-IP foundation — 2026-10-15

Read the current entries in PROJECT_STATE.md/HANDOFF.md and IP_LIFECYCLE_20261015.md. Runtime ab92f7f/schema175 is verified on both control planes; full IP lifecycle remains incomplete. This delta supersedes the older source-approved publication-pending entry below.

---

# Additional-IP continuation — 2026-10-15

Read IP_LIFECYCLE_20261015.md and development-jobs/ip-lifecycle-20261015.json first. Source-approved read-only foundation; full lifecycle is incomplete and disabled. Reconcile publication/runtime receipts before continuing. Earlier generated indexes remain historical, not evidence of this unindexed delta.

---

# Operational repair continuation — 2026-10-08

Read OPERATIONAL_REPAIR_DELTA_20261008.md first. It supersedes the archived frontier below. Verify live canonical/runtime and the integration status before continuing; do not infer deployment from a source checkpoint.

---

# Project Brain

This is the unified retrieval layer for a fresh ChatGPT session. It combines the line-level source snapshot, semantic symbol relationships, route index, schema/migration index and test map without requiring broad source reopening.

Use: `python3 tools/project-brain-query.py '<query>'`.

Supported direct forms include `internal/adminapi/server.go:105`, symbol/handler names such as `createAccount`, endpoint fragments such as `/api/v1/output`, table names such as `accounts`, file/path fragments, and exact text fragments. Results are commit-pinned and include the source commit.

Retrieval order for a fresh session: Project Brain query -> semantic/symbol/schema/test indexes -> exact line snapshot -> live source/runtime only when freshness or deeper behavior verification is required. Never answer a line-number question without binding it to the snapshot/source commit.
