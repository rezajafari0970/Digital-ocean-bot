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

Pending exact-candidate release verification and sequential production installation. Account credentials, selected zones, existing profile settings and client identities must be compared before and after installation. This correction does not create a production profile or DNS records on the user's behalf.
