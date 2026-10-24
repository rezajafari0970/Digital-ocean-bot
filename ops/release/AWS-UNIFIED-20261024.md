# AWS regional catalog and onboarding correction

Baseline private source: 06739aed93ba3dc8e6a89daee749079d543115b0.
Baseline public stable and both instances: 27257387077ff80e2e2a4d7539bc61d3bb7a2e06.

The screenshots exposed absent pricing, a regional-only account catalog,
duplicate optional selections, an implied bundled SSD, and a distant network
approval. The AWS factory now composes up to five separately scoped regional
drivers. Existing regional identities remain compatible. Explicit additional
regions pin their public subnets and managed network intent before save.
Inventory and identity search fail closed on any regional error. Network
journals are validated globally and cleaned up within their exact regions.

The catalog admits current x86_64 HVM/EBS types offered in the chosen subnet's
availability zone and compatible with all offered Ubuntu boot/ENA requirements.
Create revalidates the selected image and plan before any compute mutation.
EBS is separate storage: at least 12 GiB or the image minimum, whichever is
larger. The AWS UI adapter shows the instance ID, GiB/MiB RAM, CPU and a regional
compute price rather than inventing included SSD capacity.

Optional pricing uses the official AWS GetProducts endpoint, Linux shared
On-Demand filters, complete bounded pagination, positive finite USD hourly
rates, dated terms and conflict rejection. It is bounded to 12 seconds,
20 pages and 16 MiB. Compute-only estimates are not invoices or spending
approvals. Missing pricing permissions and unavailable prices remain explicit.
The account catalog cache remains credential- and version-scoped.

The mobile wizard retains scheduling edits across network reload, offers
None for optional plan/image choices, removes duplicate defaults, and places
explicit network consent beside Save. No automatic consent is introduced.

Validation so far: AWS race suite and mobile browser checks (authorization
popup available and blocked) pass. Full normal Go tests/vet and installer
unit checks pass. The final 156-case isolated database/native/race gate,
secret scans, exact candidate hosted installation and rollout preservation
checks are required before promotion. Final evidence is appended after execution.

A read-only production count found zero saved AWS accounts. Synthetic wire
and browser fixtures are not a live customer-account test. The legacy host
remains quarantined; no builds or database operations are run there.

OpenAI API: 0 new calls. Prior cumulative ledger: 3 calls, 6,809 tokens,
USD 0.05716284695 estimated; three-call cap already consumed.
