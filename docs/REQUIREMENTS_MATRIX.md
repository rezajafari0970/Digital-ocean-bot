# Requirements Matrix

| Area | Invariant / intended behavior | Primary code ownership |
|---|---|---|
| Provider core | Provider-neutral Registry/Factory; no DO/Vultr branches in generic lifecycle | `internal/providers`, `internal/app` |
| DigitalOcean | Mature baseline stays working; provider-specific catalog/errors/pagination inside driver | `internal/providers/digitalocean` |
| Vultr | Independent read/mutation/capacity semantics; no DO clone | `internal/providers/vultr` |
| Vultr console | Human-interactive challenge/noVNC path isolated from generic provider core | `internal/providers/vultrconsole`, Admin API |
| Account network | Per-account isolated proxy identity; required proxy failure is fail-closed | `internal/accounts`, `internal/network`, `internal/app` |
| Lifecycle | One coherent state-machine owner; reconciled/observable stages | `internal/droplets`, worker/app lifecycle |
| Sanaei | API-driven live panel operations where implemented; SSH not Output read plane | `internal/panels/sanaei` |
| Reality | Policy-driven VLESS/Reality ports/SNI/users/quota/lifetime/device limits | `internal/panels/policy`, Reality modules |
| Output | Fast/live canonical set; exact visibility/expiry; share/direct rules match | `internal/adminapi/output*`, Sanaei runtime |
| Residential | Separate panel data-plane routing from account control-plane proxy | `residentialsync`, `adminapi/residential.go` |
| UI | Fast operational module pages; preserve user context; interactive challenge feedback | `web/static/*`, `internal/adminapi` |
| Continuity | Git/source/runtime truth + docs; test/deploy/checkpoint each completed stage | repo docs/deploy workflow |
