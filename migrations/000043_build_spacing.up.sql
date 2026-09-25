ALTER TABLE accounts ADD COLUMN IF NOT EXISTS build_spacing_minutes INTEGER NOT NULL DEFAULT 60;
UPDATE accounts SET build_spacing_minutes=GREATEST(1,auto_interval_seconds/60);
