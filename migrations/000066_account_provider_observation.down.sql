ALTER TABLE accounts DROP COLUMN IF EXISTS provider_checked_at;
ALTER TABLE accounts DROP COLUMN IF EXISTS provider_state_at;
ALTER TABLE accounts DROP COLUMN IF EXISTS provider_error_detail;
ALTER TABLE accounts DROP COLUMN IF EXISTS provider_error_state;
ALTER TABLE accounts DROP COLUMN IF EXISTS provider_state_detail;
ALTER TABLE accounts DROP COLUMN IF EXISTS provider_state;
