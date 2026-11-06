# Provider browser rendering and desktop release — 2026-11-06

Accepted at 2026-11-06T04:36:13.968513+00:00. This entry supersedes older browser runtime and resource descriptions.

## Exact release
- Product source: 47a561e0df2e8076191bf4f8486b48669a047a87; canonical merge: c615da67058ae8a01a901ea80870e01ab29f3bc2 (PR45).
- Public main/stable and both runtime instances: 84aa4e4474060ac82061b83d783a70d8d6b8bb6d; schema 206 unchanged.
- Exact source CI 42160898598: 373 mandatory roots, zero skips; native isolated browser, database/race and disposable restore passed.
- Exact public CI 42161056214: source checks and Ubuntu install/reinstall passed. Atomic promotion 42161923359 passed with source/candidate pins and baseline leases.
- Both instances passed 18 preservation comparisons, five readiness checks and three running-binary hashes. Configured HTML and 27 panel assets match; shared browser runner hash verified.

## User-visible behavior
Provider websites open in a separate panel page using full Chrome for Testing 156.0.8078.47 on an isolated virtual display. The pinned runtime archive SHA256 is e6c94490f0b6b337225aeb2d2a5034858516b7017356980ba94f5176d8774a50. The legacy headless shell remains only for test compatibility.
Lossless PNG is the default, with JPEG 95 only above 8 MiB. Client density is honored up to 3x within 4 million pixels. Identical frames are not retransmitted. Desktop uses 1280 CSS pixels; mobile follows the available viewport. Mode and backend session survive wrapper reload, and closing the parent panel does not close the separate page.
Pointer phases, mouse hover/drag and touch are forwarded as native input. Chrome keeps its real user-agent and Client Hints; profile settings supply language. No stealth script, generated human activity, automatic CAPTCHA solver or fingerprint spoof was added.
At capacity, the same logged-in admin can explicitly close one of its own sessions to open another. Other operators' sessions are neither shown nor closed. Navigation stays disabled until startup; failed closure remains visible and polling continues.
The device browser only receives pixels and sends controls to the panel. Provider documents, cookies and browser storage stay in isolated disposable profiles. Fixed proxy IP/country and credential/configuration fences remain fail-closed. Concurrent route requests share only an in-flight verification; later requests recheck.

## Corrected failures and evidence
- Cold full Chrome and heavy provider pages hit soft memory limits, causing executable-page refaults and 20-second frame timeouts. Each session now has 768 MiB soft / 896 MiB hard memory. A shared systemd slice keeps the unchanged 1280 MiB combined hard budget, 1152 MiB aggregate soft limit, two-session cap, 150% total CPU and 512 total tasks. Native CI verifies the kernel aggregate cgroup limit. Private real-site observations at this budget loaded complete DigitalOcean/UpCloud pages; subsequent DigitalOcean frames were around 200–300 ms. This is an observation, not a universal latency guarantee.
- The old eight-chunk wire queue closed slow tunnels and could discard tail bytes at EOF. Bounded per-stream credits now pause a slow receiver without blocking controls or unrelated streams; EOF drains in order. A 3.2 MB asset with a 200 ms receiver pause reproduces the old truncation and passes after the fix. Capabilities negotiate per tunnel, with legacy directions tested for rolling upgrades and rollback.
- A redundant UA override suppressed real Client Hints. It was removed; native request headers, navigator metadata and profile language are verified.
- Browser route share locks previously blocked proxy-health UPDATEs and triggered the control-worker watchdog. PR44 uses FOR UPDATE SKIP LOCKED with a two-second persistence bound, preserves the prior observation when busy and retries next cycle. Credential/configuration locks are retained. Both PostgreSQL persistence and lock-concurrency tests are mandatory.
- The OVH 404 entry was corrected to its official homepage in PR41. PR42 supplied full Chrome, high-DPI/Desktop/input/session recovery. PR43's 512/640 MiB per-session limits are superseded by the bounded shared budget above.
- The earlier PR44 production update stopped before publication on retention workspace. Only owned, reproducible Chrome download/preview cache was removed. The staged candidate was reverified and resumed through the original integrity/backup gates. No account data or backups were manually purged.

## Live acceptance
Production acceptance opened the provider website from an existing Linode account entry through the selected IT proxy, checked high-DPI mobile/Desktop/resize, reload, parent closure, zero device-side provider requests and explicit End. Temporary audit credentials were removed. Control-worker readiness was checked with browser usage.

| Provider | Start / frame HTTP | Observed title or error | Challenge title detected |
| --- | --- | --- | --- |
| vultr | 201 / 200 | Log In to your Vultr Account - Vultr.com | False |
| digitalocean | 201 / 200 | Log in to your account / DigitalOcean | False |
| linode | 201 / 200 | Welcome! - Linode | False |
| upcloud | 201 / 200 | UpCloud – Login | False |
| aws | 201 / 200 | Amazon Web Services Sign-In | False |
| kamatera | 201 / 200 | Kamatera Cloud Platform - Login | False |
| ovh | 201 / 200 | Cloud Computing e hosting web / OVHcloud Italia | False |

Actual PNG captures from all seven providers were visually inspected: login forms or homepage content, fonts, controls and available artwork rendered. Provider cookie-consent dialogs were left for the user. OVH was checked in a supplementary anonymous session through the same country after the main UI run selected Linode. No visible challenge-error page was present in the reviewed captures.

These are anonymous website-rendering observations. They are not account-login or provider-API credential validation. Screenshots must be reviewed for visible page errors; a title alone does not prove the whole login flow.

## Boundaries and continuation
This remains an isolated remote browser: the device address bar is the panel URL and the provider URL appears inside the page. Exact pixel identity across OS/fonts/GPU, physical Android behavior and universal Cloudflare/hCaptcha acceptance are not guaranteed. No provider-account login or physical Android test was performed.
Session limits remain 15 minutes, two minutes of background inactivity, no file upload/download, and explicit keyboard controls for cross-origin frame inputs. Human users complete any required security challenge themselves.
Evidence: /root/dob-browser-transfer-20261106. Both final deployment intents completed; do not replay them. Existing credentials, country priorities, account and transport policies, listener ownership and schema were preserved. The damaged original cluster/duplicate workers remain quarantined; do not replay unknown-create receipts. Additional-IP work stays paused. Earlier cleanup-statistics, bulk cleanup removal and provider diagnostics remain included; no live bulk purge or cloud-server mutation was performed in this work.
