# Cloudflare rotation form correction — 2026-10-20

The screenshot schedule (2 records, 30-minute interval, 45–60-minute lifetime) needs a ceiling of at least 6 active records per server. The form previously retained the default ceiling of 4 and the API returned only a generic schedule error.

## Change

- Shared cloudflareschedule validation is used by profile admission and the DNS scheduler. The established limits and overlap reserve remain enforced.
- Changing record count, interval or lifetime raises insufficient form capacity when the resulting schedule fits the limit of 8. Explicitly entering an insufficient ceiling displays the exact required value.
- Impossible schedules explain which settings can be changed. Field-level browser validation and the submission handler both prevent invalid requests.
- Inactive Cloudflare controls are disabled and cannot block a non-Cloudflare profile.
- API errors retain their stable code and include detail, field and required_active. Browser assets have new cache versions.

## Validation before publication

- Race-enabled tests passed in internal/cloudflareschedule, internal/cloudflare, internal/panels/configdriver and internal/adminapi.
- Policy cases compare API driver and worker behavior, including the screenshot, fractional-generation overlap, impossible capacity, zero inputs and integer overflow boundaries.
- Isolated database test rejects ceiling 4 without persisting a profile and saves the same schedule with ceiling 6.
- 13 Chromium mobile cases passed, including automatic 4-to-6 adjustment, invalid manual capacity, impossible intervals, saved edit state and disabling Cloudflare with invalid hidden values.
- 7 release-gate verifier tests passed. The two new Go regressions are mandatory publication gates.

## Rollout

Completed at 2026-10-20 11:51 UTC (15:21 Tehran).

- Source correction: c016673e5ce3eb8356e1634946a24be10a73db58.
- Public release: 8c79a81652a50072498deb595072eb7cc286f45d, installed on both panels.
- Publication: 62 mandatory regressions passed with zero skips, full Go tests and vet, installer tests, 6 output tests, isolated exact-candidate installation, exact source verification and private-secret scans.
- Both panels: 3 service executable hashes match the release; all 5 readiness checks pass; no service restarts.
- Primary: all 2,872 existing profile ownership entries and 96 Trojan ownership entries are unchanged. Account settings, selected Cloudflare zones, encrypted credential hashes, client keys, profile policies and Google Ads selective routing are preserved.
- Authenticated primary API and both versioned form assets match the installed source.
- Primary final observation: all 3 existing profiles ready on 66 eligible servers, 396 published config links, output HTTP 200 in 38 ms. These are a point-in-time observation, not a guaranteed latency.
- No production Cloudflare profile or DNS records were created by this correction. The existing Cloudflare account and selected zone remain ready for the user to save the desired profile.
- Rollout evidence on each host: /root/backups/dob-cloudflare-schedule-20261020/.
- Publication evidence: /var/lib/datacenter-publisher/release-ijuflq3u/.
