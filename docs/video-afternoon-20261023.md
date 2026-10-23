# AWS route pagination and afternoon video follow-up — 2026-10-23

Status: implementation and isolated validation passed; production rollout and live database/SSH investigation are blocked by the primary controller's management connection being offline. Do not mark this task complete.

## Evidence and root cause

The screenshot shows AWS InvalidParameterValue at discovery / DescribeRouteTables, HTTP 400, local trace 3ce975ba41a44352. The 49.203-second video was reviewed across all 98 half-second samples; its audio is silent. It also shows database needs attention, two current SSH/install failures, and one waiting installer.

The deployed public cfdedbf2e0416e12dcaac87b72a01ec3592fa1b9 sends MaxResults=1000 for DescribeRouteTables. AWS documents a maximum of 100:
https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeRouteTables.html

The exact installed client.go blob equals private source 7cbe6b0d98db83c3e1718577fb966cfc93b269b5 (c1ba7d6068c8287078172a98bc5cd2485d6713be). A strict HTTP fixture reproduces InvalidParameterValue before the patch. This change specifies pagination per operation, uses 100 for route tables and instance types, keeps existing bounded sizes for the other current operations, and rejects an unregistered paginated action before making an HTTP request. NextToken handling, filters, complete inventory reads, signing, credentials and network safety checks are preserved.

## Regression acceptance

Three new mandatory tests:
- TestAWSPaginationOperationContracts: ten current paginated operations, valid bounds, GET-only semantics, filters and exact continuation tokens across two pages.
- TestAWSDiscoveryFollowsRouteTablePageWithValidLimit: full read-only onboarding with an AWS-like 400 for an oversized page and the usable subnet's route on page two.
- TestAWSUnregisteredPaginationFailsBeforeRequest: a new unregistered action cannot inherit a guessed default page size.

The tests fail on the original source and pass after the repair. All AWS package race tests pass. All 141 required database/race regressions pass, with zero skips, in a disposable socket-only PostgreSQL under an isolated unprivileged systemd user. Full go test ./..., go vet ./..., Python bootstrap/update/host-protection tests, Node checks/tests and shell syntax checks pass as of 2026-10-23T15:35:07+03:30. No exact-candidate installation or production deployment of this patch has occurred yet.

The first full isolated run failed because this separate host lacked geosite.dat required by an existing native-Xray routing test. The failure is retained. The test-only dependency was fetched from v2fly/domain-list-community release 20261023064708 and SHA-256 verified as 76b0704d7f2630e9b06d83595e9ffa363c946a1a3e19c40c0f43aa9f5d77f399; the complete run then passed. No live routing or production data was changed to satisfy the test.

## Remaining live work

At 15:34 Tehran, the primary public /readyz returned not_ready with database_recovery_required; all four worker/lifecycle progress checks were OK. This proves a real readiness alarm, not its storage or database root cause. Remote Desktop last saw the primary several hours earlier; the secondary host's available SSH key is not authorized on the primary. The primary and secondary still run public cfdedbf.

Video records requiring live investigation:
- Vultr Usman 12: SSH 167.179.74.73:22 connection refused, attempt 3, recorded 15:10:28 Tehran.
- Vultr Usman 32432: SSH unexpected packet in response to channel open: <nil>, attempt 3, recorded 15:09:08 Tehran.
- Linode usman8483: WAITING_INSTALLER, recorded 15:06:41 Tehran.
- Dashboard: active 80, tracked 93, provisioning/inactive 4, awaiting deletion 9; 80+4+9=93, with no observed broken/degraded/panel-attention count. These transition counts alone do not establish a counting defect.
- Two capacity-full accounts and scheduled observations are not evidence that existing servers are broken. Source deliberately requires a fresh read before creation; retain this guard.

Resume by reconnecting desktop-commander.service on serverprojects.ptr.network. Read the canonical handoff, current branch, integrity report and last-check log, PostgreSQL checksum/error history and kernel/storage evidence before any acknowledgement or database change. Investigate the exact still-live provision runs and their SSH/installer state. Do not suppress the database alarm, discard records, clear counters, weaken readiness, loosen SSH identity checks or declare a physical root cause without evidence. After live repairs and preserved-data verification, integrate the feature branch into the authoritative checkpoint branch, run the normal publisher with the three additional mandatory tests, install sequentially on both hosts and verify service hashes/readiness/preservation. A real AWS form retry remains required for account onboarding acceptance; do not claim EC2 creation has passed.

## Continuation locations

Secondary work copy: /root/projects/dob-video-20261023-afternoon (public-export base; the GitHub feature commit is based on the private canonical parent).
Evidence on limited-lime.ptr.network: /root/backups/dob-video-20261023-afternoon.
Isolated validation state: /var/lib/dob-afternoon-validation; completed unit: dob-afternoon-validation-20261023.service.
Original private checkpoint remains 7cbe6b0d98db83c3e1718577fb966cfc93b269b5 until integration. No paid cloud resource was created and no credential was captured. Direct OpenAI model API usage in this continuation: 0 requests, 0 tokens, USD 0.
