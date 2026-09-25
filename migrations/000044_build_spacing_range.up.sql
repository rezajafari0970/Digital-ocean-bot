ALTER TABLE accounts ADD COLUMN IF NOT EXISTS build_spacing_max_minutes INTEGER;
UPDATE accounts SET build_spacing_max_minutes=build_spacing_minutes WHERE build_spacing_max_minutes IS NULL;
ALTER TABLE accounts ALTER COLUMN build_spacing_max_minutes SET NOT NULL;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS next_build_at TIMESTAMPTZ;
