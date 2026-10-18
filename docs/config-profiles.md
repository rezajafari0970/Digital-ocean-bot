# Independent config profiles

Use **Configs → New** to create a named profile. Protocol, network, security,
port, server scope, routing classes and credential count belong to that profile.
Multiple profiles can coexist on one server. An empty port reserves a free port;
a specified port can be any supported, unoccupied port, including ports below
1024. SSH, certificate challenge, management and existing listener ports remain
reserved.

The initial catalog contains VLESS with TCP/XHTTP/gRPC Reality or TCP/WS/gRPC
TLS; VMess with TCP/WS/gRPC and optional TLS; Trojan with TCP/WS/gRPC TLS; and
Shadowsocks TCP using AES-128-GCM. The catalog permits tested combinations only.
Current official Xray clients reject plain Trojan to public addresses. The
private-network capability is documented in the catalog but cannot be selected
for public cloud profiles. Removing TLS from a link does not change its server.

## Automatic lifecycle

Saving a profile queues reconciliation. Eligible current and future servers are
enrolled according to scope. The worker persists encrypted credentials and
ownership before creating an inbound, checks port reservations and native state,
and records the result. Updates preserve credentials. A lost native response
is resolved by observing the same owned inbound; it never authorizes a blind
second POST.

Verification checks the pinned running core, its listening socket, active user
membership and an authenticated HTTPS round trip for each routing class. The controller also
checks reachability of the published port, independently of the node-local probe.
TLS profiles also require a valid publicly trusted server-IP certificate.
Certificates renew automatically. Runtime probes use the allowed destination for each route class. Eight supervised verification lanes share the existing panel-work budget; native writes release their shared executor before read-only traffic proof. Uncertain SENT writes recover by observation without queuing behind unrelated writers. A fresh routing proof, enabled account,
eligible server and current verified profile revision are required for output.
A saved port/transport change immediately fences the previous revision from
new output until the change verifies. Existing imported clients need to refresh
their subscription. Failure on one profile does not grant permission to alter
siblings or to advertise an unverified profile.

Disabling a profile stops creation and removes it from output. It preserves
already imported users and their live listeners. This is a pause, not deletion;
verified cleanup remains the explicit destructive operation.

## Compatibility with existing configurations

Migration 184 adopts the existing Trojan policy, listener ownership, secret
references and published snapshot revisions without replacing passwords or
native listeners. The old Trojan worker yields to the profile controller.
Existing Reality policies, listener pins, quotas, lifetimes and capacity rotation
remain under their existing controller. They are accessible under existing
Reality policies; transport history and manual client maintenance are advanced
diagnostics rather than steps required for new profile creation.

New profile credentials currently have no per-client quota or expiry; they
follow server lifetime. Protocol and credential membership are immutable after
creation. Use another independent profile for a different protocol or membership;
port, compatible transport, name, scope and enabled state remain editable.
Do not infer that a disabled profile's native port is free.

## Adding drivers

The pure `internal/panels/configdriver` registry composes Protocol, Transport and
Security interfaces. Register a driver and its supported capabilities, then
verify canonical server payloads, native-panel adaptation, share-URI round trips,
runtime probes and routing support. The UI reads the catalog, so a new capability
does not require another creation form. Drivers are compiled, reviewed modules;
this is not a facility for uploading arbitrary executable plugins.

The `configplane` controller owns durable desired/applied revisions, credentials,
journal, scheduling and admission. The panel adapter owns Sanaei-specific field
mapping (for example Shadowsocks per-client `security` versus Xray `method`).
Output is a separate consumer of verified state.

## Release evidence

The required regression gate runs in a disposable, socket-only PostgreSQL as an
unprivileged user. It verifies profile port admission, uncertain-write recovery,
legacy adoption, output revisions, routing classes, country labels and existing
Reality/cleanup safeguards. It also downloads or reads the pinned core, verifies
both archive and binary hashes, and performs real client/server URI round trips.
Missing or skipped required tests block publication.

For this change, the installed Sanaei panel additionally passed creation,
independent port change and fresh runtime verification for all 16 public
combinations in a disposable container. Production rollout evidence and progress
are recorded separately in the development job.

The final rollout also isolates scheduler admission and billing observations from the bulk recovery queue, preserving the five-operation control budget and 24-connection SQL pool. Production stage results and explicitly retained limitations are in [the development job](development-jobs/config-drivers-20261018.json).
