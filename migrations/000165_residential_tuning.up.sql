ALTER TABLE residential_performance_experiments ADD COLUMN tuning jsonb;
ALTER TABLE residential_performance_targets ADD COLUMN tuning_id uuid;
ALTER TABLE residential_performance_targets ADD COLUMN tuning_generation bigint NOT NULL DEFAULT 0;
CREATE INDEX residential_tuning_targets ON residential_performance_targets(experiment_id,tuning_id) WHERE tuning_id IS NOT NULL;
