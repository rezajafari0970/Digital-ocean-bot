# Native asynchronous create recovery

A provider can accept a billable create and expose its server several minutes later. Polling that operation is not a new create attempt. Previously a three-attempt workflow budget could fail a deployment before Kamatera returned the owned UUID, release the desired-count reservation and retire the late server as an orphan.

The workflow now consults durable operations fenced by account, deployment and regional idempotency key. Running, verifying and unknown attempted creates retain their deployment reservation. A succeeded operation with a resource ID also protects recovery of a lost workflow acknowledgement. No terminal deployment is revived.

Pending reconciliation preserves the charged attempt count, polls at a persisted 30-second interval and remains visible as CREATING. SQL failures do not prove absence. Real authentication, capacity and transport errors still apply when no dispatched operation remains. Definitive failure ends the waiting exemption; unresolved outcomes can require operator investigation rather than another billable request.

Every uncharged admission is explicitly observation-only. It cannot reserve or dispatch a new create, including when a concurrent observation proves a regional rejection and another region becomes eligible. A fresh dispatch requires a normal budgeted admission. Existing intent, UUID/name/region ownership, run-lease and optimistic-version fences remain enforced.

An accepted provider job accompanied by an ambiguous pending result does not by itself mark account connectivity as broken. Actual transport/authentication errors retain their callbacks, and an accepted job is never reported as a verified server.

Regression coverage includes more than ten pending polls, an already-exhausted count, account/deployment/key isolation, post-dispatch bookkeeping failure, delayed native identity, lost workflow acknowledgement, cancellation version conflicts and an uncharged regional-fallback race. These tests run against the disposable release database; no production database is used for test fixtures.

Operational recovery preserves the old failed-deployment cleanup path. Do not manually revive old failed deployments or release a temporary build hold until their owned resources have reached verified cleanup and the corrected runtime is ready.
