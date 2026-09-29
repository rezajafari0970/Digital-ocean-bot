# Step 2 Dependency Order

1. **P0-identity [BLOCKER]** — Give Vultr a stable canonical Account.ID/external_id — depends: none
2. **P0-validation [BLOCKER]** — Make image/options validation provider-catalog driven — depends: P0-identity
3. **P0-capacity [BLOCKER]** — Honor LimitKnown=false across admission and lifecycle — depends: P0-identity
4. **P0-resource-provider [BLOCKER]** — Mirror resources using owning account provider, not digitalocean literal — depends: none
5. **P0-ui-canonical [BLOCKER]** — Make wizard provider metadata/canonical ID driven and allow Vultr when promoted — depends: P0-identity, P0-validation
6. **P1-pagination [HARDENING]** — Implement Vultr pagination for instances/catalog collections — depends: none
7. **P1-secret-lifetime [HARDENING]** — Align Vultr credential retrieval/wipe/rotation with DO — depends: none
8. **P1-errors [HARDENING]** — Expand Vultr canonical error taxonomy and provider-neutral UI messages — depends: none
9. **P1-observation [HARDENING]** — Re-certify Observe/snapshot after identity/capacity/pagination changes — depends: P0-identity, P0-capacity, P1-pagination
10. **P2-contract-tests [CERTIFICATION]** — Bring Vultr contract/regression coverage to DO-equivalent behaviors — depends: P0-validation, P0-capacity, P0-resource-provider, P1-pagination, P1-secret-lifetime, P1-errors
11. **P2-promote [CERTIFICATION]** — Promote metadata development -> ready only after all offline certification passes — depends: P2-contract-tests, P1-observation
12. **P3-live-cert [LIVE]** — Run real Vultr account create/adopt/get/delete/SSH/provision/panel/output E2E — depends: P2-promote
