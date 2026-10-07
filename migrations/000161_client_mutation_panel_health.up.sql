CREATE TABLE client_mutation_panel_health (
 panel_id uuid PRIMARY KEY REFERENCES panel_instances(id) ON DELETE CASCADE,
 state text NOT NULL CHECK (state IN ('COOLDOWN','QUARANTINED')),
 failures integer NOT NULL CHECK (failures > 0),
 retry_after timestamptz,
 last_job_id uuid REFERENCES client_mutation_jobs(id) ON DELETE SET NULL,
 reason_code text NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK ((state='COOLDOWN' AND retry_after IS NOT NULL) OR (state='QUARANTINED' AND retry_after IS NULL))
);
ALTER TABLE client_mutation_execution_gate
 ADD COLUMN last_failure_code text NOT NULL DEFAULT '',
 ADD COLUMN last_failure_at timestamptz,
 ADD COLUMN last_failure_panel_id text NOT NULL DEFAULT '',
 ADD COLUMN last_failure_job_id text NOT NULL DEFAULT '';
