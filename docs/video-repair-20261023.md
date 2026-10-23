# Video repair — 2026-10-23

Source evidence: Screenrecorder-2026-10-23-07-13-53-755.mp4, 182.28 seconds, silent audio. Visual timeline inspected across the whole recording, including saving at 01:28–01:34, empty OVH output at 01:46–01:50, deployment history at 02:10–02:12, pending VM ownership at 02:18–02:22, build history at 02:46–02:50 and residential pool at 02:58–03:02.

## Reproduced defects and correction

- Repeated OVH installation failures were in `bootstrap-kernel-memory-guard`, not account authentication. The initial Ubuntu 26.04 image used `root=LABEL=cloudimg-rootfs`; update-grub generated `root=UUID=...` for the same mounted root (both independently resolved to `/dev/sda1`, device 8:1). The preservation check now accepts only a LABEL/UUID alias whose uncached blkid results are unique and resolve through lsblk to the current findmnt root device. All other kernel arguments and kernel paths remain exact. Conflicting, missing, ambiguous and different-device evidence still blocks. Original journals and GRUB backups are retained. No direct reboot was issued by the diagnostic.
- Save confirmation depended on an extra account-list read after a successful PUT. The PUT now returns the exact desired target and rule state read inside the committing transaction. The UI confirms that receipt, does not manufacture status from submitted fields, and does not replay a write when list refresh fails. Subsequent page reads retain their normal recovery behavior.
- Provider inventory can expose a VM before local create confirmation installs its lifecycle row. The dashboard now identifies that specific read-only transition with its exact account/deployment create-operation journal. It remains unmanaged and cannot be adopted or deleted through this display correction. Unknown, conflicting, failed, retired and cross-account identities remain foreign.
- Browser acceptance reproduced `accounts_regions_failed` for a legitimate JSON-null inventory array. Region extraction now handles unavailable/null inventory without breaking unrelated saved account status; non-array malformed data is not silently accepted.

## Verification before publication

The new kernel alias regression failed before the correction and passed afterwards, together with existing kernel guard race tests. Both new account receipt and pending-ownership database regressions failed before and passed after. Five database/race acceptance tests passed on a disposable socket-only PostgreSQL. The actual mobile/desktop Chromium account flow passed after an initial retained null-inventory failure, including an injected post-save GET outage, exactly one PUT, OFF/ON/shrink behavior, persistence and reload. The isolated null-inventory database test passed. Mandatory publisher regressions expand from 130 to 134 and include executable save-feedback tests.

## Live limits and acceptance pending

Two previously failed builds reached normal retirement before repair could complete; this task did not directly delete provider servers or reset user policy. A newly created canary accepted the verified boot preparation and advanced to panel installation without a new guard failure. Final installation, native boot proof, outputs, release revision and both-host verification will be appended after publication.

Unknown provider-wide quota, historical errors, provisioning waits and unavailable external proxy endpoints must remain truthful; they are not forced healthy. The saved OVH target 10, lifetime 993–1006 minutes, spacing 39–96 minutes, selected regions/plans/OS and apply-to-existing OFF are preserved.

An incidental corrupted diagnostic Go cache object was retained and replaced only from a separately hash-verified matching object. The physical cause of historical cache/storage corruption remains unestablished. Evidence is under `/root/backups/dob-video-repair-20261023` on the primary host. No OpenAI model API request was made in this continuation.
