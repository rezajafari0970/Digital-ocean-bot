ALTER TABLE accounts ALTER COLUMN build_spacing_max_minutes SET DEFAULT 60;
UPDATE accounts SET build_spacing_max_minutes=COALESCE(build_spacing_max_minutes,build_spacing_minutes,60);
