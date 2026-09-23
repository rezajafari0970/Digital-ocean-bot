ALTER TABLE schedules ADD COLUMN lease_until TIMESTAMPTZ;
CREATE INDEX schedules_lease_idx ON schedules(next_run_at,lease_until) WHERE enabled=true;

ALTER TABLE deployments ADD COLUMN lock_version BIGINT NOT NULL DEFAULT 0;
ALTER TABLE operations ADD COLUMN lock_version BIGINT NOT NULL DEFAULT 0;
