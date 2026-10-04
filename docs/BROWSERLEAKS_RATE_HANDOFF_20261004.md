# BrowserLeaks routing and rate input — continuation

Production API and worker use clean source commit `3d362e5a3e6a8de55c0443cfd5f87bc444871502`. This later checkpoint is documentation only.
Canonical path and branch are unchanged. Recheck live HEAD/origin, policies and gate before new work.

- Residential protected domains now include `domain:browserleaks.com`: root plus subdomains, alongside existing advertising categories.
- Direct identities retain Direct priority. Protected traffic does not fall back to server egress when residential is unavailable.
- The Residential form includes the categories and a BrowserLeaks IP test link. Connect with a Residential config before opening it.
- Config creation speed is a dropdown of integers1–100, preserving existing saved values. It controls credential generation, not connection concurrency. Raw API validation remains strict.
- No migration, policy parameter change, DNS catalog change or credential rotation was performed.

Verification: focused, race, full, clean detached and actual-Xray fault tests passed. Mobile browser verified selector,
saved rate preservation, save payloads, invalid-request rejection and the test link. Independent Sanaei running-route
proofs covered38 panels and456 class/network/domain cases; two transient pending samples passed on targeted fresh reads.
User Direct/Residential settings remained unchanged during deploy.

The first release added two explicit probes to every routine readiness verification. Production canary exposed deadline
overruns on some panels. The final release retains the prior bounded representative set; content-addressed outbound tags
still prove the full loaded settings. BrowserLeaks-specific acceptance remains independent. See JSON evidence for the
final serving-panel state and post-repair sample.

Upstream UDP/QUIC capability, external browser traffic bypassing the tunnel, and30000 concurrent-user capacity are not
certified by these route-selection checks. Preserve existing fail-closed behavior and previously recorded external blockers.

Evidence: `/root/backups/dob-browserleaks-rate-20261004`, with final build/deploy under `v2`.
