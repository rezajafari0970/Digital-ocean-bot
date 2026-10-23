# AWS pagination, controller protection and database recovery — 2026-10-23

Final software release f548fb6115e896e8cca07fd2b958df7daccacdcf, from private product source 024f24bd3927c410b3e50d0443d107cd15e13ca8, is deployed and verified on both 85.192.63.181 and 176.124.198.111. API plus both workers match the release hashes on both hosts, all readiness checks pass, and both normal update commands exited zero. Final acceptance does not establish a physical storage root cause or a real AWS credential onboarding result.

## AWS correction

The screenshot's DescribeRouteTables InvalidParameterValue was reproduced: the old shared paginator sent MaxResults=1000 while this operation permits at most 100. Pagination is now registered per operation, unknown operations fail before HTTP, and continuation tokens/filters/signatures/network guards are preserved. Three regressions failed before the repair and passed after it, including full discovery with the usable route on page two. Official contract: https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeRouteTables.html

The complete 49.203-second video was inspected through 98 half-second samples; audio is silent. Counts shown in the video are consistent: 80 active + 4 provisioning + 9 awaiting deletion = 93 tracked. Capacity-full and scheduled-read states alone are not defects. Fresh mutation admission and account lifecycle limits remain intact.

## SSH and installer evidence

Exact runs c2f32aeb-3c3f-4d85-bc01-21d16bc37b4e (Vultr Usman 12), cd28a84c-62bb-4ef0-b257-207f692ebefe (Vultr Usman 32432), and e6703e2c-50f8-4ee0-94fa-c09a7fc943f0 (Linode usman8483) all reached READY / COMPLETED / PANEL_COMPLETE with enabled panels using their existing recovery path. Historical failures were retained. No retry state, host identity, quota or installer protection was bypassed.

## Controller memory protection

Both controllers had default-enabled KHO on Ubuntu kernel 7.0, no kho=off, and CmaFree greater than CmaTotal=0. The existing managed-node guard did not cover controllers. Its Python implementation is now a shared embedded asset, preserving the previous remote command bytes exactly; controller installation, explicit preparation, activation proof and independent five-minute monitoring reuse that asset. Read-only preflight blocks susceptible unprotected hosts. Containers report external kernel ownership and never change host boot settings. Neither the installer nor routine monitor reboots a machine automatically.

Both controllers were rebooted serially with verified backups. Boot identity changed, kho=off is active, CMA counters are zero, and services returned. Secondary booted its already-installed default 7.0.0-38 kernel instead of its prior 7.0.0-34. Unrelated GRUB entries were preserved. This mitigates a demonstrated kernel risk; it is not proof that KHO caused every storage or cache corruption.

## Database recovery and limits

The first validation passed full physical pg_basebackup checksums, SHA-256 manifest/WAL verification, logical dump and strict amcheck, followed by an isolated logical restore. After the controlled primary reboot, a fresh amcheck found one damaged page of cloudflare_records, relation base/16389/442649276 block 641. The prior healthy copied page has an identical 24-byte header, LSN and stored checksum; exactly 16 bytes differ. Detection after reboot does not establish when or why those bytes changed.

Production writers and PostgreSQL were stopped cleanly. The whole current cluster was preserved before changes. Offline pg_checksums scanned 2,408 files / 236,533 blocks and found exactly this one bad page. The same-version page from the verified physical backup was restored in a separate copy first; full checksums and strict amcheck passed there. That copy contained 15,264 Cloudflare records. Only the matching 8 KiB page was then restored to the stopped original database; the actual file differences are the 16 damaged bytes, with every other byte of the relation preserved. Full checksums returned zero bad pages; strict amcheck and all Cloudflare row digests matched the isolated copy. A new logical backup was sealed and copied off-host. Aligned O_DIRECT disk readback matched the exact restored page hash ab652c8b212249f5f4645210d993ad778d927d07e1117f9e6f89a137f6cd8b73.

No whole-database rollback, row deletion, checksum bypass or statistics reset occurred. Historical checksum_failures=1 remains as the acknowledged watermark. Original corruption evidence and physical copies remain protected. Fresh encrypted off-host snapshot: d3e5acbb3f9b0496468dd77a23b8cc4735db72ae6bfcfec390970a1275573d2a.

Physical corruption origin remains unresolved. Hypervisor/RAM/ECC and virtual block/storage/cache evidence is required to attribute it. Healthy checks now and the KHO mitigation do not justify a guarantee against future infrastructure failures.

## Final acceptance

- 143 mandatory database/race regressions passed with zero skips; full Go test/vet, Python/Node/shell checks and exact Ubuntu 24 LXC installation passed. Evidence: /var/lib/datacenter-publisher/release-c083ip74.
- GitHub main run 39962024281 and stable run 39962025783 succeeded for the exact public revision.
- Master key, environment, account settings/credentials, Cloudflare settings/credential hashes, transport/profile/output/protection policies and selective advertising behavior were preserved.
- Primary profile identities: 4,444 common, 4,436 unchanged, eight independently confirmed ordinary expiry retirements, zero unexpected changes; four ordinary new identities. All 77 Trojan identities unchanged. Secondary preservation checks also passed.
- Authenticated HTTPS accounts/proxies/profiles/output endpoints returned 200. A real mobile browser rendered ten accounts and the AWS form with masked credentials, hidden save before validation, correct action caption, no horizontal overflow and no JavaScript errors. Diagnostic admin session was removed afterward.
- No stored AWS account exists. Real credential validation/EC2 creation was not attempted and no paid cloud resource was created by this repair. The next real onboarding attempt should be made through the panel; do not send secret keys in chat.

Evidence and executable verification scripts are under /root/backups/dob-video-20261023-afternoon on both hosts. Direct OpenAI model API usage for this continuation: 0 requests, 0 tokens, USD 0; this excludes ChatGPT/tool usage.
