CREATE TABLE residential_performance_experiments (
 id uuid PRIMARY KEY, spec jsonb NOT NULL, state text NOT NULL CHECK(state IN('RUNNING','KEPT','ROLLING_BACK','ROLLED_BACK')),
 version bigint NOT NULL DEFAULT 1, deadline timestamptz NOT NULL, reason text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX residential_one_open_experiment ON residential_performance_experiments((true)) WHERE state IN('RUNNING','KEPT','ROLLING_BACK');
CREATE TABLE residential_performance_panels (
 panel_id uuid PRIMARY KEY REFERENCES panel_instances(id) ON DELETE CASCADE,
 experiment_id uuid NOT NULL REFERENCES residential_performance_experiments(id),
 generation bigint NOT NULL DEFAULT 1, applied_generation bigint NOT NULL DEFAULT 0,
 config jsonb, baseline jsonb, observed jsonb, pending jsonb,
 verified_at timestamptz, last_error text NOT NULL DEFAULT ''
);
CREATE TABLE residential_performance_targets (
 experiment_id uuid NOT NULL REFERENCES residential_performance_experiments(id),
 panel_id uuid NOT NULL, before_config jsonb, generation bigint NOT NULL,
 state text NOT NULL DEFAULT 'PENDING' CHECK(state IN('PENDING','APPLIED','ROLLBACK_PENDING','RESTORED','RETIRED')),
 failures integer NOT NULL DEFAULT 0, PRIMARY KEY(experiment_id,panel_id)
);
CREATE TABLE residential_performance_operations (
 request_id uuid PRIMARY KEY, request_hash text NOT NULL, response jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE panel_routing_state ADD COLUMN performance_generation bigint NOT NULL DEFAULT 0;
