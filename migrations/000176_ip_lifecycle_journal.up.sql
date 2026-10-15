-- Additional-IP durable control plane. This migration grants no provider/network authority.
-- Ledger account identities deliberately have no cascading FK: billing evidence survives purge.
CREATE TABLE ip_policies (
 account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 family integer NOT NULL CHECK(family IN(4,6)),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 policy jsonb NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(account_id,family),
 CHECK((policy->>'family')::integer=family)
);
CREATE TABLE ip_policy_events (
 id bigserial PRIMARY KEY, account_id uuid NOT NULL, family integer NOT NULL,
 revision bigint NOT NULL, policy jsonb NOT NULL, at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(account_id,family,revision)
);
CREATE TABLE ip_plans (
 digest text PRIMARY KEY CHECK(length(digest)=64),
 account_id uuid NOT NULL, family integer NOT NULL CHECK(family IN(4,6)),
 policy_revision bigint NOT NULL, body jsonb NOT NULL,
 created_at timestamptz NOT NULL, saved_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE ip_approvals (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), plan_digest text NOT NULL REFERENCES ip_plans(digest),
 account_id uuid NOT NULL, family integer NOT NULL CHECK(family IN(4,6)),
 approved_by text NOT NULL CHECK(length(approved_by)>0),
 approved_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 expires_at timestamptz NOT NULL,
 currency text NOT NULL CHECK(currency IN('USD','EUR')),
 max_commitment_micros bigint NOT NULL CHECK(max_commitment_micros>=0),
 funded_until timestamptz NOT NULL,
 max_allocations integer NOT NULL CHECK(max_allocations>0),
 ongoing_funding boolean NOT NULL DEFAULT false,
 revoked_at timestamptz,
 CHECK(expires_at>approved_at AND funded_until>=expires_at)
);
CREATE TABLE ip_mutation_intents (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 account_id uuid NOT NULL, provider text NOT NULL,
 family integer NOT NULL CHECK(family IN(4,6)), product text NOT NULL,
 server_id text NOT NULL CHECK(server_id<>''), region text NOT NULL CHECK(region<>''),
 idempotency_key text NOT NULL CHECK(length(idempotency_key) BETWEEN 1 AND 128),
 action text NOT NULL CHECK(action IN('ALLOCATE','ATTACH','DETACH','RELEASE')),
 resource_id text NOT NULL DEFAULT '',
 approval_id uuid NOT NULL REFERENCES ip_approvals(id),
 request jsonb NOT NULL,
 commitment_micros bigint NOT NULL CHECK(commitment_micros>=0),
 state text NOT NULL DEFAULT 'PLANNED' CHECK(state IN('PLANNED','DISPATCHED','UNKNOWN','ACCEPTED','REJECTED','CONFIRMED','CANCELLED','BLOCKED')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 dispatch_id uuid UNIQUE,
 dispatched_at timestamptz,
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 next_attempt_at timestamptz,
 error_class text NOT NULL DEFAULT '',
 UNIQUE(account_id,idempotency_key),
 CHECK((dispatch_id IS NULL)=(dispatched_at IS NULL)),
 CHECK(action<>'ALLOCATE' OR resource_id='')
);
CREATE INDEX ip_mutation_account_state ON ip_mutation_intents(account_id,state);
CREATE TABLE ip_mutation_receipts (
 id bigserial PRIMARY KEY, operation_id uuid NOT NULL REFERENCES ip_mutation_intents(id),
 receipt_key text NOT NULL, kind text NOT NULL CHECK(kind IN('RESPONSE','OBSERVATION')),
 body jsonb NOT NULL, at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(operation_id,receipt_key)
);
CREATE TABLE ip_mutation_lanes (
 account_id uuid NOT NULL, family integer NOT NULL CHECK(family IN(4,6)),
 next_attempt_at timestamptz, next_dispatch_at timestamptz, rate_limit_until timestamptz, failures integer NOT NULL DEFAULT 0,
 PRIMARY KEY(account_id,family)
);
CREATE TABLE ip_allocations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), account_id uuid NOT NULL,
 provider text NOT NULL, family integer NOT NULL CHECK(family IN(4,6)), product text NOT NULL,
 provider_resource_id text NOT NULL, server_id text NOT NULL, region text NOT NULL,
 acquisition_operation uuid NOT NULL UNIQUE REFERENCES ip_mutation_intents(id),
 parent_prefix cidr, acquired_at timestamptz NOT NULL,
 state text NOT NULL DEFAULT 'OWNED' CHECK(state IN('OWNED','RELEASE_UNKNOWN','RELEASED','LOST')),
 last_observed_at timestamptz NOT NULL,
 released_at timestamptz,
 currency text, hourly_micros bigint CHECK(hourly_micros>=0),
 cost_source text, cost_checked_at timestamptz,
 CHECK((state='RELEASED')=(released_at IS NOT NULL))
);
CREATE UNIQUE INDEX ip_allocation_active_identity ON ip_allocations(account_id,product,provider_resource_id) WHERE state<>'RELEASED';
CREATE TABLE ip_endpoints (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), allocation_id uuid NOT NULL REFERENCES ip_allocations(id),
 account_id uuid NOT NULL, server_id text NOT NULL, address inet NOT NULL,
 parent_prefix cidr, diversity_prefix cidr,
 active_scope text NOT NULL CHECK(active_scope IN('GLOBAL','ACCOUNT')),
 history_scope text NOT NULL CHECK(history_scope IN('GLOBAL','ACCOUNT')),
 history_seconds integer NOT NULL CHECK(history_seconds>=0),
 policy_revision bigint NOT NULL,
 acquired_at timestamptz NOT NULL, lifetime_seconds integer NOT NULL CHECK(lifetime_seconds>0),
 expires_at timestamptz NOT NULL,
 phase text NOT NULL DEFAULT 'ALLOCATED' CHECK(phase IN('ALLOCATED','ATTACHED','NETWORK_READY','HEALTHY','PUBLISHED','DRAINING','RETIRED','LOST')),
 revision bigint NOT NULL DEFAULT 1,
 network_revision text NOT NULL DEFAULT '',
 replacement_id uuid REFERENCES ip_endpoints(id),
 retired_at timestamptz,
 CHECK(masklen(address)=CASE WHEN family(address)=4 THEN 32 ELSE 128 END),
 CHECK((family(address)=6)=(diversity_prefix IS NOT NULL)),
 CHECK(parent_prefix IS NULL OR address <<= parent_prefix),
 CHECK(diversity_prefix IS NULL OR address <<= diversity_prefix),
 CHECK(expires_at>acquired_at),
 CHECK((phase='RETIRED')=(retired_at IS NOT NULL))
);
CREATE UNIQUE INDEX ip_endpoint_active_address ON ip_endpoints(address) WHERE retired_at IS NULL;
CREATE INDEX ip_endpoint_account_server ON ip_endpoints(account_id,server_id);
CREATE TABLE ip_endpoint_history (
 id bigserial PRIMARY KEY, account_id uuid NOT NULL, server_id text NOT NULL,
 allocation_id uuid NOT NULL REFERENCES ip_allocations(id), endpoint_id uuid REFERENCES ip_endpoints(id),
 address inet NOT NULL, parent_prefix cidr, diversity_prefix cidr,
 active_scope text NOT NULL, history_scope text NOT NULL, history_seconds integer NOT NULL,
 result text NOT NULL, at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE ip_endpoint_proofs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), endpoint_id uuid NOT NULL REFERENCES ip_endpoints(id),
 endpoint_revision bigint NOT NULL, network_revision text NOT NULL,
 panel_id uuid NOT NULL, config_revision bigint NOT NULL, port integer NOT NULL CHECK(port BETWEEN 1 AND 65535),
 route_class text NOT NULL CHECK(route_class IN('DIRECT','RESIDENTIAL')),
 provider_attached boolean NOT NULL, guest_configured boolean NOT NULL,
 protocol_ok boolean NOT NULL, route_isolation_ok boolean NOT NULL,
 user_network_ok boolean NOT NULL, fallback_ok boolean NOT NULL,
 observed_at timestamptz NOT NULL, expires_at timestamptz NOT NULL,
 evidence_digest text NOT NULL CHECK(length(evidence_digest)=64),
 CHECK(expires_at>observed_at)
);
CREATE TABLE ip_publication_leases (
 endpoint_id uuid NOT NULL REFERENCES ip_endpoints(id), panel_id uuid NOT NULL,
 client_id text NOT NULL, uri_digest text NOT NULL CHECK(length(uri_digest)=64),
 route_class text NOT NULL CHECK(route_class IN('DIRECT','RESIDENTIAL')),
 port integer NOT NULL CHECK(port BETWEEN 1 AND 65535),
 config_revision bigint NOT NULL,
 first_published_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 last_published_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 valid_until timestamptz, -- NULL is an unbounded real client lease, never visibility TTL.
 migrated_at timestamptz, migration_receipt text,
 PRIMARY KEY(endpoint_id,panel_id,client_id,uri_digest),
 CHECK((migrated_at IS NULL)=(migration_receipt IS NULL))
);
CREATE TABLE ip_native_delete_fences (
 account_id uuid NOT NULL, server_id text NOT NULL,
 dispatched_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(account_id,server_id)
);
CREATE TABLE ip_network_footprints (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), endpoint_id uuid NOT NULL REFERENCES ip_endpoints(id),
 revision text NOT NULL, previous_revision text NOT NULL,
 snapshot_digest text NOT NULL CHECK(length(snapshot_digest)=64),
 plan_digest text NOT NULL CHECK(length(plan_digest)=64),
 state text NOT NULL CHECK(state IN('PREPARED','APPLIED','UNKNOWN','RESTORED','CONFLICT')),
 at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(endpoint_id,revision)
);
CREATE FUNCTION ip_reject_ledger_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'IP ledger is append-only' USING ERRCODE='23514'; END $$;
CREATE TRIGGER ip_receipts_immutable BEFORE UPDATE OR DELETE ON ip_mutation_receipts FOR EACH ROW EXECUTE FUNCTION ip_reject_ledger_change();
CREATE TRIGGER ip_history_immutable BEFORE UPDATE OR DELETE ON ip_endpoint_history FOR EACH ROW EXECUTE FUNCTION ip_reject_ledger_change();
CREATE TRIGGER ip_policy_history_immutable BEFORE UPDATE OR DELETE ON ip_policy_events FOR EACH ROW EXECUTE FUNCTION ip_reject_ledger_change();
CREATE TRIGGER ip_plan_immutable BEFORE UPDATE OR DELETE ON ip_plans FOR EACH ROW EXECUTE FUNCTION ip_reject_ledger_change();
CREATE FUNCTION ip_account_obligations(a uuid, server text DEFAULT '') RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM ip_mutation_intents m WHERE m.account_id=a AND (server='' OR m.server_id=server)
  AND m.state NOT IN('REJECTED','CANCELLED','CONFIRMED'))
 OR EXISTS(SELECT 1 FROM ip_allocations i WHERE i.account_id=a AND (server='' OR i.server_id=server) AND i.state<>'RELEASED')
 OR EXISTS(SELECT 1 FROM ip_endpoints e WHERE e.account_id=a AND (server='' OR e.server_id=server) AND e.retired_at IS NULL)
$$;
CREATE FUNCTION ip_guard_account_purge() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ip_account_obligations(OLD.id) THEN RAISE EXCEPTION 'IP obligations block account purge' USING ERRCODE='23514'; END IF;
 RETURN OLD;
END $$;
CREATE TRIGGER ip_account_purge_guard BEFORE DELETE ON accounts FOR EACH ROW EXECUTE FUNCTION ip_guard_account_purge();

-- Immutable identities prevent a stale worker repurposing a previously authorized operation.
CREATE FUNCTION ip_freeze_intent() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'IP intent retention required' USING ERRCODE='23514'; END IF;
 IF (to_jsonb(NEW)-ARRAY['state','dispatch_id','dispatched_at','updated_at','next_attempt_at','error_class'])
 IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','dispatch_id','dispatched_at','updated_at','next_attempt_at','error_class'])
 OR (OLD.dispatch_id IS NOT NULL AND (NEW.dispatch_id IS DISTINCT FROM OLD.dispatch_id OR NEW.dispatched_at IS DISTINCT FROM OLD.dispatched_at))
 THEN RAISE EXCEPTION 'IP dispatch identity immutable' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER ip_intent_identity BEFORE UPDATE OR DELETE ON ip_mutation_intents FOR EACH ROW EXECUTE FUNCTION ip_freeze_intent();
CREATE FUNCTION ip_freeze_approval() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR (to_jsonb(NEW)-'revoked_at') IS DISTINCT FROM (to_jsonb(OLD)-'revoked_at')
 OR (OLD.revoked_at IS NOT NULL AND NEW.revoked_at IS DISTINCT FROM OLD.revoked_at)
 THEN RAISE EXCEPTION 'IP approval immutable' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER ip_approval_identity BEFORE UPDATE OR DELETE ON ip_approvals FOR EACH ROW EXECUTE FUNCTION ip_freeze_approval();

-- Executable evidence is immutable and explicitly scoped; policy/preview booleans are not evidence.
CREATE TABLE ip_lifecycle_inventory_receipts (
 request_id uuid PRIMARY KEY, account_id uuid NOT NULL,
 started_at timestamptz NOT NULL, finished_at timestamptz NOT NULL, observation jsonb NOT NULL
);
CREATE TRIGGER ip_lifecycle_inventory_immutable BEFORE UPDATE OR DELETE ON ip_lifecycle_inventory_receipts FOR EACH ROW EXECUTE FUNCTION ip_reject_ledger_change();
CREATE TABLE ip_execution_evidence (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 account_id uuid NOT NULL, credential_generation text NOT NULL,
 provider text NOT NULL, family integer NOT NULL CHECK(family IN(4,6)),
 product text NOT NULL, region text NOT NULL, server_id text NOT NULL,
 inventory_request_id uuid NOT NULL REFERENCES ip_lifecycle_inventory_receipts(request_id),
 body jsonb NOT NULL, observed_at timestamptz NOT NULL, valid_until timestamptz NOT NULL,
 superseded_at timestamptz,
 CHECK(valid_until>observed_at)
);
CREATE UNIQUE INDEX ip_execution_evidence_current ON ip_execution_evidence(account_id,provider,family,product,region,server_id) WHERE superseded_at IS NULL;
CREATE TABLE ip_capacity_evidence (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), account_id uuid NOT NULL,
 scope_key text NOT NULL, credential_generation text NOT NULL,
 inventory_request_id uuid NOT NULL REFERENCES ip_lifecycle_inventory_receipts(request_id),
 family integer NOT NULL CHECK(family IN(0,4,6)), product text NOT NULL DEFAULT '',
 region text NOT NULL DEFAULT '', server_id text NOT NULL DEFAULT '',
 resource_limit integer CHECK(resource_limit>=0), used integer CHECK(used>=0),
 included_resources jsonb NOT NULL DEFAULT '[]',
 observed_at timestamptz NOT NULL, valid_until timestamptz NOT NULL, source text NOT NULL,
 superseded_at timestamptz,
 CHECK(valid_until>observed_at)
);
CREATE UNIQUE INDEX ip_capacity_evidence_current ON ip_capacity_evidence(account_id,scope_key) WHERE superseded_at IS NULL;
CREATE TABLE ip_account_funding (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), account_id uuid NOT NULL UNIQUE,
 revision bigint NOT NULL DEFAULT 1, currency text NOT NULL CHECK(currency IN('USD','EUR')),
 budget_micros bigint NOT NULL CHECK(budget_micros>=0),
 funded_until timestamptz NOT NULL, max_concurrent integer NOT NULL CHECK(max_concurrent BETWEEN 1 AND 8)
);
ALTER TABLE ip_approvals ADD COLUMN funding_scope uuid REFERENCES ip_account_funding(id);
ALTER TABLE ip_approvals ADD COLUMN funding_revision bigint;
CREATE TABLE ip_operation_reservations (
 operation_id uuid NOT NULL REFERENCES ip_mutation_intents(id),
 scope_key text NOT NULL,
 PRIMARY KEY(operation_id,scope_key)
);
CREATE TABLE ip_funding_reservations (
 operation_id uuid PRIMARY KEY REFERENCES ip_mutation_intents(id),
 funding_scope uuid NOT NULL REFERENCES ip_account_funding(id),
 currency text NOT NULL, commitment_micros bigint NOT NULL CHECK(commitment_micros>=0),
 funded_until timestamptz NOT NULL, settled_at timestamptz
);
CREATE TABLE ip_resource_incidents (
 id bigserial PRIMARY KEY, allocation_id uuid NOT NULL REFERENCES ip_allocations(id),
 inventory_request_id uuid NOT NULL REFERENCES ip_lifecycle_inventory_receipts(request_id),
 kind text NOT NULL, at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(allocation_id,inventory_request_id,kind)
);
CREATE TRIGGER ip_incident_immutable BEFORE UPDATE OR DELETE ON ip_resource_incidents FOR EACH ROW EXECUTE FUNCTION ip_reject_ledger_change();

CREATE FUNCTION ip_invalidate_network_proofs() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.state='APPLIED' AND NEW.state<>'APPLIED' THEN
 UPDATE ip_endpoints SET revision=revision+1,phase=CASE WHEN phase IN('HEALTHY','PUBLISHED') THEN 'NETWORK_READY' ELSE phase END WHERE id=NEW.endpoint_id AND network_revision=NEW.revision;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER ip_network_proof_invalidation AFTER UPDATE ON ip_network_footprints FOR EACH ROW EXECUTE FUNCTION ip_invalidate_network_proofs();

CREATE FUNCTION ip_freeze_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR (to_jsonb(NEW)-'superseded_at') IS DISTINCT FROM (to_jsonb(OLD)-'superseded_at')
 OR (OLD.superseded_at IS NOT NULL AND NEW.superseded_at IS DISTINCT FROM OLD.superseded_at)
 THEN RAISE EXCEPTION 'IP execution evidence immutable' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER ip_execution_evidence_immutable BEFORE UPDATE OR DELETE ON ip_execution_evidence FOR EACH ROW EXECUTE FUNCTION ip_freeze_evidence();
CREATE TRIGGER ip_capacity_evidence_immutable BEFORE UPDATE OR DELETE ON ip_capacity_evidence FOR EACH ROW EXECUTE FUNCTION ip_freeze_evidence();
CREATE TABLE ip_funding_events (
 id bigserial PRIMARY KEY, account_id uuid NOT NULL, revision bigint NOT NULL,
 body jsonb NOT NULL, at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(account_id,revision)
);
CREATE TRIGGER ip_funding_events_immutable BEFORE UPDATE OR DELETE ON ip_funding_events FOR EACH ROW EXECUTE FUNCTION ip_reject_ledger_change();

-- Preserve the exact encrypted credential/AAD identity for scoped cleanup after rotation.
CREATE TABLE ip_cleanup_credentials (
 operation_id uuid PRIMARY KEY REFERENCES ip_mutation_intents(id),
 account_id uuid NOT NULL, original_id text NOT NULL, kind text NOT NULL,
 ciphertext bytea NOT NULL, nonce bytea NOT NULL, key_version integer NOT NULL
);
CREATE FUNCTION ip_guard_cleanup_credential() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' OR ip_account_obligations(OLD.account_id) THEN
  RAISE EXCEPTION 'IP cleanup credential is retained for unresolved obligations' USING ERRCODE='23514';
 END IF;
 RETURN OLD;
END $$;
CREATE TRIGGER ip_cleanup_credential_guard BEFORE UPDATE OR DELETE ON ip_cleanup_credentials FOR EACH ROW EXECUTE FUNCTION ip_guard_cleanup_credential();
