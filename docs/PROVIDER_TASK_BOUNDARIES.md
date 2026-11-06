# Provider and task boundaries

The provider integrations are compiled modules. Registration is explicit at `internal/providers/builtin`; there is no arbitrary dynamic code loading or plugin sandbox. `app` owns durable account, network, mutation and publication policy. Drivers own provider protocols and report the stores required to implement them.

| Boundary | Contract | Failure behavior |
| --- | --- | --- |
| Built-in registration | Seven factories; normalized unique identity; metadata copied on registration and read | Nil factories, inconsistent identities and registration panics are rejected without panic values |
| Driver opening | Context, named factory, actual capabilities and interface implementation | Cancelled calls do not start; nil or incompatible drivers never reach the account runtime |
| Runtime stores | Optional `RuntimeRequirementProvider` declares external identity, resources, absence proof, plans and cleanup journal | Missing declared storage or saved external identity blocks the runtime |
| Account network | Profile must reference the same healthy proxy; guarded client required for proxy-only accounts | No implicit default transport for a proxy-required runtime; invalid route is not repaired by direct fallback |
| Proxy health | At most 16 KiB; one complete JSON value and validated egress observation | Oversized, partial, trailing or cancelled observations are not healthy |
| Short task | `resilience.Guard` at supervised work, recovery and batch boundaries | Panic becomes a sanitized error; no implicit retry and no fabricated remote success or absence |
| Module lifecycle | Each registered module receives its own cancellable context, including after early sibling failure | Sibling cancellation and bounded joining happen before the role returns; systemd owns process restart |
| Recovery | Existing durable checkpoint and failure ledger | Panic is persisted with backoff; unknown native outcome still needs reconciliation |
| Output | Plugin collector exclusively owns plugin-prefixed listeners; legacy collector requires a nonempty string client ID | Legacy metadata cannot resurrect withheld plugin output; malformed IDs are not exported |

## Adding a provider

1. Implement `Factory`, immutable metadata, and `Driver`. Advertise only interfaces actually implemented. Register the factory in `builtin.NewRegistry`.
2. Declare only the durable dependencies the driver needs through `RuntimeRequirementProvider`. Saved-account opening supplies account-scoped stores; onboarding remains a separate lifecycle without a saved external identity.
3. Use the supplied HTTP client and credential source. Do not create a default transport, override the assigned route, log credentials, or turn an unknown network response into proof of remote absence.
4. Preserve existing idempotency journals and ownership/credential/generation fences for mutations. Provider-specific mutation classification, account diagnostics and regional authorization still have explicit reviewed adapters in `app`; moving registration does not remove these safety policies.
5. Add isolated protocol, authorization, pagination, cancellation, error-classification and ambiguous-outcome tests, and opt critical regression roots into the mandatory release gate. Cloud creation/deletion is not a test prerequisite.

## Task ownership

Task guards are containment boundaries, not a process-wide guarantee. A module must honor its context and clean up resources it owns. Goroutines started inside a callback still need their own ownership and error boundary. Module initialization always enters the registered lifecycle so that early sibling failure cannot skip cleanup. Native mutation failures remain subject to the existing durable reconciliation rules; a panic does not authorize replay.

## Verification scope

The structural inventory covers 38 top-level internal modules, 659 production Go files and 632 test files at the audited revision. This is an import/test inventory, not a claim of manual line-by-line review of every file. Full automated suites exercise the repository; the new manual work concentrated on provider factories, runtime dependencies, account routing and health, task supervision/recovery and output ownership. Live checks only cover saved active accounts. No zero-bug, 99-percent primary-IP availability, CAPTCHA bypass or universal provider acceptance claim is made.
