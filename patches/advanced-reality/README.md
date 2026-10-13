# Advanced Reality server forks — are-0.2.1 (experimental)

These patches are an opt-in server experiment. They are not installed on the production fleet. The existing Bot observation API still cannot apply fleet changes.

## Pinned source and licenses

See `manifest.json` for the complete upstream commits, patch hashes, Node archive hash and Go toolchains. The three source trees are Xray 26.9.9, its pinned REALITY dependency, and Sanaei 3.10.0. Each upstream LICENSE is retained alongside the corresponding patch. A complete corresponding source tree is obtained by checking out the recorded commit and applying its recorded patch. The build tool does exactly that; preserve these sources, licenses, notices and build instructions with distributed binaries.

## Implemented behavior

* Xray's server-side TCP/REALITY wrapper counts started, successful, failed and timed-out REALITY handshakes, rejected admissions, in-flight pre-authentication work and live authenticated REALITY connections. REALITY success is **not** VLESS UUID authentication or a count of users.
* Optional policies limit concurrent pre-authentication work to 1–512 and its duration to 1–30 seconds. These bounds are experimental controls, not recommended universal settings. The deadline includes dialing the target and unauthenticated fallback forwarding. A forwarded browser connection may therefore also be closed at that limit; this behavior requires separate network evaluation.
* The REALITY dependency observes caller cancellation in target I/O and its post-handshake-record wait. An established successful connection does not retain the pre-authentication deadline.
* Drain rejects new REALITY admissions while established connections finish. It does not migrate connections, reload inbounds, replace binaries without interruption or terminate old connections by age.
* State transitions require the process instance, exact generation and request ID. Repeating the exact request returns its historical receipt; callers must fetch current state after an unknown outcome. Conflicting or stale requests fail. The in-memory journal admits at most 256 distinct mutations per process.
* Policies are leased for 15–300 seconds and revert to the disabled baseline on expiration. This is enforced on the next admission/state/control operation. Restart creates a new instance and resets policies; old instance requests are rejected.
* Linux socket ownership uses a process-lifetime file lock and a private ownership record. Crash recovery only removes a refused socket matching the recorded device, inode, ctime and UID. Active, unknown, corrupted or ambiguous ownership is rejected. This is not proof of recovery from disk corruption or every power-loss timing.
* Sanaei adds authenticated `GET /panel/api/server/advancedReality` and `POST /panel/api/server/advancedReality/transition`. These use existing admin/session and CSRF protection. Monitor/node-sync tokens cannot mutate this surface. The relay has bounded request/response sizes and timeouts, and never falls back to restarting Xray. A transport error on POST returns `outcome_unknown: true`.

## Activation and API

The Xray process must receive both `XRAY_ARE_SOCKET=/absolute/private-dir/control.sock` and `XRAY_ARE_PORTS=24445`. The existing parent directory must be private (0700). The socket and ownership file are private (0600). At most 16 explicit ports are accepted. A scope becomes writable only when an actual TCP REALITY listener registers it.

Sanaei uses `XUI_ARE_SOCKET` to reach that socket. An unset variable leaves the endpoint disabled. There is no new client JSON field or client binary. The existing server/client JSON shape, UUID, key, SNI, protocol authentication and routing are not changed by a control transition.

A transition body contains `instance`, `port`, `expected_generation`, `request_id`, `lease_seconds` and `policy`. Policy contains `enabled`, `max_handshakes`, `timeout_ms`, and `drain`. Read state immediately before deciding. Rollback sends a new CAS transition containing the saved previous policy; it does not replay a stale receipt as if it were current. If another writer changes the instance/generation, stop and reconcile. There is no implicit retry.

## Build and critical verification

On a Linux amd64 build host with Git, Python supporting safe tar extraction, Go toolchain download support and a C compiler:

```sh
python3 tools/build-advanced-reality-forks.py --work-dir /absolute/new-build-directory
```

The tool downloads exact source commits and the checksum-pinned Node archive, applies checksum-pinned patches, builds the real Sanaei frontend, builds both binaries and runs the mandatory race/native tests. It installs no services and opens no production listener. It fails on missing mandatory tests, skips or failures. Receipts and binaries remain in the specified directory.

The native panel test uses an isolated database, a real modified Xray process, the actual authenticated controller, a TCP deadline, rollback, unchanged configuration bytes and SIGKILL/restart recovery. The server-scope tests verify admission limits, concurrent accounting, lease expiry, CAS/idempotency, request bounds, permissions, live-owner exclusion and refusal to remove foreign sockets.

## Evidence boundaries

The initial 30-minute WAN data-path run uses build `are-0.2.0` (hash in the manifest). Version 0.2.1 adds socket lifecycle recovery, close synchronization, descriptor handling and native crash tests. These are separately built and tested; the earlier soak is not relabeled as a 30-minute run of the final binary.

The phone baseline was reported successful by the user on MCI and Irancell. That report concerns the stock 26.9.9 profile, not the modified binary. Exact installed phone core, long phone sessions and network switching remain unverified.

This version does not implement XHTTP admission control, TLS record-size tuning, traffic padding/fragmentation, new TLS/cipher algorithms, a persistent fleet mutation worker, seamless binary replacement, full-host reboot validation or rollout across all Ubuntu/core versions. No performance increase or DPI invisibility is claimed. Full fleet activation remains blocked.
