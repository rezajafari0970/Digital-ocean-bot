DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM residential_performance_experiments WHERE tuning->>'phase' IN('TESTING','PUBLISHING','RESTORING')) THEN
 RAISE EXCEPTION 'Complete tuning recovery/application before removing its durable state'; END IF;
END $$;
DROP INDEX residential_tuning_targets;
ALTER TABLE residential_performance_targets DROP COLUMN tuning_generation, DROP COLUMN tuning_id;
ALTER TABLE residential_performance_experiments DROP COLUMN tuning;
