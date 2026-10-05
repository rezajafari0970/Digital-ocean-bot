DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM residential_performance_experiments WHERE duration_mode='permanent' AND state IN('RUNNING','KEPT','ROLLING_BACK')) THEN
  RAISE EXCEPTION 'Roll back and verify permanent profiles before removing publication support';
 END IF;
END $$;
ALTER TABLE residential_performance_experiments DROP CONSTRAINT residential_performance_fleet_permanent;
ALTER TABLE residential_performance_experiments DROP CONSTRAINT residential_performance_deadline_mode;
UPDATE residential_performance_experiments SET deadline=COALESCE(deadline,updated_at);
ALTER TABLE residential_performance_experiments ALTER COLUMN deadline SET NOT NULL;
ALTER TABLE residential_performance_experiments DROP COLUMN publish_scope,DROP COLUMN duration_mode;
