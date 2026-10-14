-- Read-only IP discovery foundation. No policy, scheduler or paid operation is enabled.
CREATE TABLE ip_inventory_lanes (
 account_id uuid PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
 state text NOT NULL DEFAULT 'NOT_STARTED' CHECK(state IN ('NOT_STARTED','READING','OBSERVED','FAILED')),
 request_id uuid,
 lease_until timestamptz,
 started_at timestamptz,
 finished_at timestamptz,
 next_attempt_at timestamptz,
 rate_limit_until timestamptz,
 failures integer NOT NULL DEFAULT 0 CHECK(failures>=0),
 error_class text NOT NULL DEFAULT '',
 http_status integer NOT NULL DEFAULT 0,
 last_good jsonb,
 last_good_at timestamptz
);
CREATE TABLE ip_inventory_events (
 id bigserial PRIMARY KEY,
 account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 request_id uuid NOT NULL UNIQUE,
 started_at timestamptz NOT NULL,
 finished_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 state text NOT NULL CHECK(state IN ('OBSERVED','FAILED','SUPERSEDED')),
 error_class text NOT NULL DEFAULT '',
 http_status integer NOT NULL DEFAULT 0,
 retry_after_seconds bigint NOT NULL DEFAULT 0 CHECK(retry_after_seconds>=0),
 rate_limit_until timestamptz,
 observation jsonb
);
CREATE INDEX ip_inventory_events_account ON ip_inventory_events(account_id,id DESC);
CREATE TABLE ip_quota_evidence (
 account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 family integer NOT NULL CHECK(family IN (4,6)),
 product text NOT NULL,
 scope text NOT NULL,
 current_limit integer CHECK(current_limit>=0),
 used integer CHECK(used>=0),
 observed_at timestamptz NOT NULL,
 source text NOT NULL,
 last_known_limit integer CHECK(last_known_limit>=0),
 last_known_at timestamptz,
 last_known_source text,
 capacity_changed boolean NOT NULL DEFAULT false,
 PRIMARY KEY(account_id,family,product,scope)
);
