# Profile port reuse after deletion — 26 October 2026

Recorded 2026-10-26T02:20:14.172246+03:30.

## Observed problem

The attached 9.38-second recording shows a new VLESS/TCP/NONE profile on port
443 rejected with a port-reserved error. Configs shows only the existing
Tcp reality 8443 profile; the earlier deleted profiles are already hidden.
The same recording shows publication counts recovering from 96/101 to 99/101;
this is not evidence of a permanently frozen catalog.

Read-only production diagnosis confirmed that one listener belonging to the
previously deleted Reality profile still reserved 443 on an unavailable AWS
panel. Both the admission query and schema-197 guard treated this reservation
as a fleet-wide prohibition. Against the same live rows, the original query
returned conflict=true and the revised query returned conflict=false. Exactly
one panel retained the pending reservation. No diagnostic profile was inserted.

## Result and safety boundaries

A committed deletion now releases its desired fleet-wide port reservation.
Replacement profiles may be saved and proceed on independent destinations.
Applied listeners and PREPARED/SENT mutation reservations continue to block the
same port on their own panel until native absence or provider termination is
confirmed. The worker checks this ownership before credential preparation and
again under the shared panel writer lock before remote changes. Affected
panels report profile_port_cleanup_pending and retry independently.
Observation-only SENT recovery is preserved.

The API permits only precisely owned pending-deletion inventory to be deferred:
panel, remote ID, managed key and applied port must match. Active desired
profiles, paused-but-not-deleted owners, unknown/disabled inbounds, panel ports
and legacy authority retain their guards. Explicit and automatic port selection
use this policy. Migration 198 updates the database guard without removing any
listener, client, secret, or audit evidence. Its down migration refuses a
rollback that would conflict with queued replacement profiles.

## Delivery and verification

- Source e2e8dc85b4f3c6be1b6a0f46f5f65059eb46354a; PR16 merged as 603afc762b12207aa751c575319dd14d31236860.
- Public main/stable and BOTH live instances: 30bff5efb645da3fbac72acfb782d144fc2b32ac; schema 198.
- Source CI 40238650286: 234 required roots passed, zero required skips;
  Go/static, real browser, database/native/race and isolated restore/interruption.
- PostgreSQL coverage includes invalid credentials; explicit/automatic reuse;
  CONFIRMED/PREPARED/SENT owners; independent panels and ports; exact inventory
  matching; active, paused and manual conflicts; legacy mode; rollback refusal.
- The existing native lifecycle now saves a replacement before old cleanup,
  proves it cannot change the occupied native port, and creates it only after
  confirmed removal. This strengthens panel safety while removing global delay.
- First run 40237966566 caught missing deployments.host in the new fixture.
  The fixture was completed in e2e8dc85 and the full gate rerun; no assertions
  were removed or timeouts extended. Other isolation/native tests passed then.
- Public CI 40238717523 passed source checks and fresh/repeated install.
  Atomic promotion 40239159812 verified the exact candidate and leases.
- Full export scans passed against 39032 private production values (none logged),
  default-instance values and gitleaks. The later fixture-only delta was scanned
  and every unchanged export hash checked (export2-scan-validation.json).
- Both final accepted updates exited zero; each passed all 15 preservation checks,
  three running artifact hashes and five readiness checks. HTTPS version/assets
  and normal TLS validation matched. No pending deployment; DB checksums zero.
- The installed database guard body matches migration 198 exactly. Read-only
  live admission on 443 reports conflict=False.
  Panels retaining native cleanup reservation: 1.

Two production update attempts stopped before publication at the unchanged
insufficient_retention_workspace guard; the previous release remained healthy.
Both failed attempts and logs are preserved under production-rollout/attempt-1
and attempt-2. Reproducible apt metadata and Go module caches first reclaimed
about 203 MB. This was insufficient after the fresh backup/archive cycle.
Next, six superseded production runtimes, twelve obsolete installer source
copies, and three earlier archive files were preserved on the existing archive
host. Four transferred archive files total 512357402 bytes; source/destination
SHA256 checks and local tar comparisons passed before local removal. Durable
pointers identify every archived path. Active runtimes, nearest rollback,
current staged candidate and all fresh/incident backup files were unchanged.
Post-relocation free space was 7811604480 bytes. The exact already-built
candidate was resumed through the original integrity, space, publication and
migration guards, without rebuilding or relaxing checks.

The first post-install ownership check incorrectly required an installed
Cloudflare certificate for a normal Reality profile activation. Read-only
inspection found one native-confirmed initial activation eight seconds after
the baseline and well before deployment, plus five confirmed expired-server
retirements. All six changes affected only remote_id; credentials and secret
hashes remained identical. The verifier now requires a Cloudflare certificate
only for profiles that select Cloudflare, while retaining matching native
confirmation/spec/revision/event/time and unchanged credential/hash checks.
The original failure, verifier and observation remain in verification-attempt-1;
the baseline was not replaced, product code was not changed, and deployment was
not repeated. The corrected preservation check and final live acceptance passed.

## Remaining boundaries and next action

Active profiles: 1.
Background cleanup profiles: 3.
Unconfirmed native removals: 3.

No new production profile was created as a test, and the user's existing
Tcp reality 8443 settings/ownership were preserved. The user can save the
replacement on 443; the unavailable panel continues waiting for cleanup without
holding up the rest of the fleet. The visible plain-TCP client compatibility
warning was unchanged; official Android client acceptance was not measured.

Fresh pre-update backups and private receipts are under
/root/dob-port-reuse-20261026. The previous routine 617270497-byte dump was copied
to the existing archive host, both content hashes verified and a durable pointer
written before reclaiming its original space. Current/incident archives remain.
The original corrupted cluster/services remain quarantined. Its physical root
cause and the comprehensive remaining module audit are still unresolved.
No new paid OpenAI request was sent; prior unknown usage accounting remains open.

Do not repeat the accepted deployment. Continue remaining audit work from its
inventory; restore authorized remote reachability to finish the pending cleanup.
