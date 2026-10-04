# Plain Output, residential UDP and blocked cloud cleanup — 2026-10-04

Continue from /root/projects/Digital-ocean-bot-canonical-e2e on serverprojects.ptr.network, branch checkpoint/final-e2e-20260929. Production is authoritative. Check clean HEAD == origin before editing. This checkpoint supersedes the unresolved-UDP conclusion in ADS_ONLY_AND_ACCOUNT_PURGE_HANDOFF_20261004.md; that earlier acceptance remains a historical record.

## Runtime and acceptance

- API: 500098755e5520846c80dff2836c3c591a48a324, clean detached build.
- Worker: add8e110cfcb3efdb72ebbc47fe9469a5c22693d, unchanged; no worker restart was needed.
- A documentation-only checkpoint advances canonical HEAD beyond these runtime revisions intentionally.
- Acceptance: docs/PLAIN_OUTPUT_UDP_CLOUD_ACCEPTANCE_20261004.json.
- Evidence: /root/backups/dob-plain-udp-cloud-20261004.

Both services are active. Latest acceptance: 38 active servers, zero broken servers, 38 DIRECT and 38 RESIDENTIAL fresh disjoint Output identities. All 38 panel routing proofs are fresh/APPLIED at revision 376. Global Reality remains enabled at revision 23. Main mutation gate remains enabled at concurrency 1; legacy bulk gate remains closed.

## Output completed

The opt-in live browser Share view displays only full VLESS URI lines in a plain preformatted block. Heading, counts, Copy button, status and error text were removed. Each URI occupies one line; horizontal scrolling preserves long lines. Refresh remains approximately once per second. Error/empty/hidden states clear the old content. No HTML/error response is rendered as a configuration.

Raw subscription URLs remain text/plain. Token-bound route class, freshness, expiry and selection filters are unchanged. Actual mobile Chromium acceptance verified body visible text equals the configuration block, with no heading/status/button, and all lines are VLESS URIs.

Source: internal/adminapi/output_view.go and web/static/output-live.js. Regression coverage: web/tests/output_live.cjs and web/tests/admin_regression.cjs. Focused, race, full and clean detached Go tests passed. Only API binary and output-live.js were deployed.

## Res1 UDP root cause fixed and tested

Res1 endpoint: 1507f2c1-1f56-467d-a079-f5b438bdd671, private.residential.proxyrack.net:10000, SOCKS5.

Observed sequence:
1. Original username/sticky assignment: authenticated, UDP ASSOCIATE reply 1.
2. Adding ;udp=true to that existing sticky assignment: still reply 1.
3. Adding ;udp=true plus a fresh named session: reply 0 and valid UDP DNS response.
4. An isolated Xray client completed actual QUIC HTTP/3 requests and responses through that upstream, including adservice.google.com.

The tested username suffix ;udp=true;session=dobudp20261004 is now saved on Res1. The existing admin update/test path was used, with fresh post-read, lost-response handling and account-proxy isolation checks. No account-network proxy was modified.

Provider primary instructions:
- https://help.proxyrack.com/en/articles/8433702-target-udp-ips
- https://help.proxyrack.com/en/articles/5977249-create-multiple-sessions-on-a-sticky-port

The real production VLESS canary 69e68a2a-8a42-4c9f-9f12-302920d729bc then passed both Output classes:
- Non-ad HTTPS exits through the server IP.
- UDP DNS resolves successfully.
- QUIC HTTP/3 www.google.com returned 200 with a response body.
- QUIC HTTP/3 adservice.google.com returned 404 with a response body (successful network/TLS/H3 exchange).
- Running-core routeTest independently proved ad TCP/UDP uses residential for RESIDENTIAL clients and direct for DIRECT clients; non-ad destinations remain direct.
- All 38 serving panels have fresh matching route proofs.

Keep the domain lists and ads-only behavior from the preceding implementation. Do not revert to all-traffic residential. No claim is made that every advertising hostname is classifiable, every panel has individually performed the full H3 traffic test, or the upstream can never fail. Residential IP speed/health selection remains the user's deferred task.

## Cloud cleanup is NOT completed

The four accounts remain absent from the operational DB. Their credentials/resource identities were read from the existing pre-purge backup in an isolated, bounded recovery probe; no account was restored and no provider DELETE was sent. The task-created temporary selected encrypted archive was removed after recording a non-secret scope manifest.

Fresh reads through each account's saved proxy establish:
- Three DigitalOcean accounts: account endpoint returns status=locked; inventory and each of the five exact droplet GETs return HTTP 422 with an account-lock message directing the owner to provider support.
- One Vultr account: account and inventory return HTTP 401 Unauthorized; 15 previously tracked instances remain unverified.

Exact account/resource scope and dated responses are in the acceptance JSON. There are 20 unverified resources, not 20 confirmed extant resources and not 20 confirmed deletions.

Required next input is provider-authorized access: unlock the three DigitalOcean accounts and place a valid authorized Vultr API credential on the server using the normal secure mechanism. Never request that the user paste credentials into chat. Once access works, use complete fresh inventory and exact-resource checks, then scoped durable deletion with read-before-write recovery and final absence verification. Use the production provider drivers for pagination and deletion; the temporary diagnostic probe was read-only.

Historical backups/system journals have not been securely erased. Do not erase the only remaining recovery credentials or claim full cloud/backup erasure while provider cleanup is blocked. Existing operational purge acceptance and other live accounts remain unchanged.
