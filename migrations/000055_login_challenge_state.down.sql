ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_browser_challenge_type_check;
ALTER TABLE accounts DROP COLUMN IF EXISTS password_rotation_requested_at;
ALTER TABLE accounts DROP COLUMN IF EXISTS browser_challenge_detail;
ALTER TABLE accounts DROP COLUMN IF EXISTS browser_challenge_at;
ALTER TABLE accounts DROP COLUMN IF EXISTS browser_challenge_type;
