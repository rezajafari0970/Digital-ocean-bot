# Server protection — implementation and operating contract

The Config page exposes durable ON/OFF controls for selected servers or all current and future eligible servers. Migration 157 defaults OFF. Enabling requests convergence; it does not declare a server protected until its local policy revision, agent liveness, fresh metrics, nft capability and admission state are verified. No expiry timer is imposed on this profile.

## Local protection

A small Go process samples Linux memory availability, swap use, CPU/per-core saturation and steal, CPU/memory/I/O PSI, global file-descriptor use, conntrack use, disk/inode reserve and interface traffic/drop rates every 250 ms. These are local pressure observations, not a measured bandwidth capacity or per-user accounting system.

Initial thresholds deliberately reserve headroom: immediate admission closure below max(32 MiB, 4% of RAM); sustained closure after two seconds for memory pressure, CPU queue pressure, global FD/conntrack exhaustion or disk/inode pressure. Reopening requires 30 continuous healthy seconds. Missing telemetry/capabilities are explicitly unverified or unsupported.

Only new TCP SYNs to public VLESS TCP/raw ports read from the current Xray JSON are rejected during critical pressure. Port 22, the management-panel port and loopback listeners are excluded. Established connections are retained. The agent never edits Xray configuration, identities, DNS, sysctls, routes, proxy pools or existing firewall tables. It owns only the marked inet dob_guardian table. Its atomic nft transaction is fenced by the verified kernel table handle: a table replaced after inspection is not deleted by name. Blocked-set elements expire after 15 seconds unless renewed; loss of the agent cannot leave an indefinite admission lock.

A separate recovery loop checks x-ui every five seconds. It only starts inactive/failed x-ui, with sufficient fresh resource headroom and a current enabled policy. Active/activating/deactivating services are not restarted. A durable attempt ledger permits at most three starts in ten minutes, at least 60 seconds apart, under a shared recovery lock. Existing periodic diagnostics yield recovery ownership while the local policy is enabled. A healthy active x-ui does not itself prove Xray end-to-end reachability; the existing panel and Output checks remain relevant.

The guardian has its own systemd sandbox and 64/128 MiB memory high/max limits. These limits apply to the guardian only. They are not Xray capacity limits.

## Control, state and rollback

The administrator API uses expected revisions and idempotency keys. Node generations are monotonic independently of global revisions, including eligibility withdrawal/re-entry. A stale receipt cannot apply to a newer desired generation. Database advisory locks, remote install locks and local policy/agent locks serialize reconciliation. SSH handshake, session and upload cancellation are bounded without relaxing host-key pins.

The worker reconciles up to eight servers concurrently. Successful receipt polls are scheduled about every 15 seconds, with a five-second coordination loop. Policy reload is two seconds locally. UI polling is five seconds; UI verification expires after 60 seconds. Fleet convergence and recovery are not 50 ms operations. Measured isolated nft actuation is recorded separately and is not a client latency guarantee.

Fresh, current, verified critical/recovering receipts suppress that server from new Output exports for at most the receipt freshness interval. Missing/stale/failed receipts and OFF preserve existing export eligibility. Already-imported links and active TCP sessions cannot migrate to another IP through this server-side change.

Disable is a durable request covering every ever-assigned living node, including nodes no longer selected. The worker stops/disables the guardian, removes its owned admission table and verifies DISABLED with no running agent. Unreachable nodes remain pending; they are never called successfully rolled back. The binary and disabled policy remain for reversible future activation.

Before rolling back API/worker binaries or migration 157, first disable and wait for all living assigned nodes to report DISABLED at their desired revision. The down migration refuses an enabled policy or unverified cleanup. Restore backed-up binaries/static assets only after that gate. Provider resources and Xray identities are not deleted for rollback.

## Verification

Run tools/server-protection-acceptance.sh on the managed build host. It uses a separately named bulk_test database and isolated schemas, never the production database for tests. It runs the full Go unit suite, focused race tests, actual SSH blackholes, nft packet tests in a separate network namespace, agent lifecycle/recovery tests in private mount/network namespaces, PostgreSQL revision/rollback/output tests and a mobile/desktop browser flow. Production canary checks must additionally verify the actual emitted systemd unit, fresh receipts across renewal, unchanged Xray configuration/hash and unchanged x-ui PID.

API-assisted plan: resp_01817ae5f2b7c45e006ac44f4f838887d1a2e0d3ac9ac986fc.
API-assisted source review: resp_04a05ba263285624006ac45b44b13487d196d68c82f45bb67c.
Review corrections cover table-handle fencing, deployment eligibility withdrawal, stale administrator forms, stale admission labels, cleanup-pending counts and unit-only restart handling. Some review requests for missing tests referred to files not included in its prompt; those tests are present in the acceptance suite.

## Explicit boundaries

This release implements server resource admission and bounded recovery. It does not guarantee uninterrupted hardware/provider/network availability, prevent overload caused solely by existing flows, migrate live TCP sessions, provide app-side automatic failover, or enforce per-user admission. Asho app source is not in this repository. Network rates and swap use are observed; no unmeasured bandwidth ceiling, forced reboot, swap tuning or kernel OOM exemption is applied.
