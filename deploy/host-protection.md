# Independent host protection

These optional root-owned services are separate from the API, workers, installer,
Cloudflare scheduler and provider drivers. Failure of remote backup or TLS renewal
does not stop the database or rewrite application configuration.

## Jobs and trust boundaries

- `datacenter-host-evidence.timer`: every five minutes, capture checksum
  counters, integrity state, filesystem error counters, free space and kernel
  evidence. Keep a bounded ninety-day evidence history outside application logs.
- `datacenter-host-backup.timer`: every two hours, send evidence and the newest
  verified sealed daily backup to a TLS-pinned, encrypted restic repository.
  A changed checksum watermark, failed/stale integrity check, or old local backup
  prevents database publication. Existing remote snapshots remain intact.
  A confirmed manifest digest avoids duplicate database snapshots on retries.
- `datacenter-host-retain.timer`: only on the independent vault, check the
  repository and retain seven daily, two weekly and at least two database
  snapshots. Evidence keeps fourteen daily snapshots. Forensic snapshots are
  excluded from automatic retention.
- The vault uses a dedicated service account, append-only REST access, bcrypt
  authentication, a storage quota and an ingress allowlist. The application host
  cannot prune remote backups. Maintenance runs as the repository owner.
- The repository password is escrowed in a root-only file on the vault so recovery
  does not depend on the original host surviving. Transport and encryption keys
  are separate. Never put these credentials in Git or application responses.

Provision `/etc/datacenter-host-protection/{client.env,restic-password,vault.crt}`
on the client, or `restic-password` on the vault, root-owned mode 0600 inside a
0700 directory. Install restic on both hosts and an append-only rest-server on the
vault. Then run `python3 deploy/install-host-protection.py --role client` or
`--role vault`. The installer validates credentials and installs its own pinned
copy under `/usr/local/lib/datacenter-host-protection`; normal application updates
do not remove or restart it. Use `datacenter-host-protection status` as root.
Re-run the optional installer explicitly when upgrading these independent jobs.

## HTTPS ingress

Use a dedicated nginx frontend and a trusted ACME certificate, with automatic
renewal and a validated nginx reload deploy hook. For an IP certificate use a
Certbot version supporting IP identifiers and the short-lived profile. Keep
port 80 available for HTTP-01 validation. Preserve existing SSH/TLS services.

When the application's internal health endpoint must retain its original port,
redirect only external ingress using an independently named nftables table.
Keep loopback health checks unchanged. Validate TLS and redirects from another
host before considering ingress complete. Avoid query-string access logs because
subscription URLs may contain credentials. Test renewal with a staging dry run,
and test the authenticated browser flow on the HTTPS origin.

## Storage incident handling

Never turn off checksums, fsync or full-page writes to hide corruption. Do not
reset checksum statistics or acknowledge repair automatically. Preserve the
original physical evidence and verify both logical restoration and page checks
before acknowledging a repair. Evidence of changed bits does not by itself
identify RAM, a virtual disk, a host cache or a physical device. Hypervisor/ECC/
storage logs are required to distinguish those causes when the guest has no
hardware visibility.

Verification includes offline forensic checks, unit tests for unsafe/stale backup
rejection, an encrypted off-host restore with full manifest verification and a
real PostgreSQL restore, append-only DELETE rejection, and certificate renewal.
