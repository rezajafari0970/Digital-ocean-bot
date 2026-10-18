# Unified configuration lifecycle

Every configuration is a profile with its own protocol, network, security, port, server scope, routing classes, client count and desired revision. No protocol has a privileged creation path. The existing driver registry composes independently registered protocol, transport and security implementations; only combinations tested against the pinned official core are advertised publicly.

## User workflow

Create with New, select a supported combination, choose an explicit port or leave it empty for automatic reservation, set the count and scope, then Save. Saving schedules reconciliation automatically. Edit changes the same profile. Pause removes its active listeners and output while retaining identities for Resume. Delete permanently retires its users and listeners; the port is released only after confirmed absence. No manual client-maintenance or transport-apply step is needed.

The count is per routing class per server. Direct and Residential remain separate immutable client identities. Count increases retain existing users; decreases remove surplus native and global users through a durable retirement journal. Profiles follow server lifetime. Public Trojan uses TLS on the supported official core; plaintext Trojan is not advertised as a working public capability.

## Ownership and recovery

- Desired state, applied native state, runtime verification and published output are separate recorded stages.
- Every profile/server pair has an advisory lease. Panel mutations use the existing panel lock and serialized admission gate.
- Atomic due-row claims prioritize the earliest due work, then the oldest attempt, distributing retries fairly across profiles and servers. Status and enrollment use the same eligibility predicate, including servers near the end of their lifetime.
- Prepared and sent mutations freeze the exact specification, membership and credential reference. Lost responses trigger fresh observation before any further mutation.
- Native confirmation is persisted before connection proof. A slow connection test does not repeat creation or monopolize the creation lanes.
- Updates and deletion verify the owned native identity and a hash of sibling listeners. Foreign changes block only the affected operation.
- Port admission reserves desired, applied and uncertain pending ports. Native absence plus running-core socket absence is required before releasing an applied reservation.
- Removed global users remain journaled until their exact identities are observed absent. Restarting a worker does not forget unfinished cleanup.
- Failures have bounded retries and recorded stages. Runtime failure withholds that profile's output until verification succeeds.

## Statistics and interface recovery

Ready-server and published-config counts use the same publication predicate as subscriptions, in one repeatable-read database snapshot. Native verification alone does not imply a published connection. Old revisions, stale output, failed routing proof, disabled accounts and incomplete cleanup are excluded.

The browser preserves the last observed cards with an explicit stale notice when reads fail. Reads have a hard deadline, a single in-flight request per mounted page, and page-identity checks. Navigation, session expiration or late responses cannot overwrite a newer page. Controls wait for the saved revision before another action on that profile.

## Upgrade and retirement

Migration 185 adds journals and authority state without deleting existing native listeners. An existing installation activates the unified authority through the authenticated `POST /api/v1/configs/activate-unified` operation after backup and deployment verification. It records one durable cleanup job, retires previous configuration profiles and disables legacy Reality, Trojan, bulk and manual creation authorities. Database triggers prevent those legacy policies from being re-enabled.

Cleanup proceeds independently per server and retries recoverable failures. A completed server becomes eligible immediately; it does not wait for unrelated failed servers. New profile intents can be saved during cleanup. A fresh installation with no existing panels activates the single authority automatically and creates no default protocol.

## Verification evidence

The required disposable-PostgreSQL gate includes profile resize, preserved identities, pause/resume, lost mutation responses, sibling preservation, confirmed deletion, port reuse, idempotent creation, migration retention, exact publication counts, cleanup isolation and legacy-authority retirement. Native integration exercises all 16 publicly supported combinations through the pinned panel and official Xray, including authenticated traffic. Mobile Chromium tests cover the unified workflow, transport-specific paths, stale edits, failed and hung reads, navigation, authentication and layout.

External provider, network and certificate failures remain possible. The controller reports them and retries without claiming successful output or changing unrelated profiles.
