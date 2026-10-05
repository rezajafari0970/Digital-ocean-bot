ALTER TABLE residential_performance_experiments
 ADD COLUMN duration_mode text NOT NULL DEFAULT 'timed' CHECK(duration_mode IN('timed','permanent')),
 ADD COLUMN publish_scope text NOT NULL DEFAULT 'selected' CHECK(publish_scope IN('selected','fleet'));
ALTER TABLE residential_performance_experiments ALTER COLUMN deadline DROP NOT NULL;
ALTER TABLE residential_performance_experiments ADD CONSTRAINT residential_performance_deadline_mode
 CHECK((duration_mode='timed' AND deadline IS NOT NULL) OR (duration_mode='permanent' AND deadline IS NULL));
ALTER TABLE residential_performance_experiments ADD CONSTRAINT residential_performance_fleet_permanent
 CHECK(publish_scope<>'fleet' OR duration_mode='permanent');
