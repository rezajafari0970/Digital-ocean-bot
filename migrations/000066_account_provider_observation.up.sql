ALTER TABLE accounts ADD COLUMN IF NOT EXISTS provider_state text NOT NULL DEFAULT 'UNKNOWN';
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS provider_state_detail text;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS provider_error_state text;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS provider_error_detail text;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS provider_state_at timestamptz;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS provider_checked_at timestamptz;
UPDATE accounts SET provider_state=CASE
 WHEN runtime_status='PROVIDER_LOCKED' THEN 'LOCKED'
 WHEN runtime_status='PROVIDER_TOKEN_INVALID' THEN 'TOKEN_INVALID'
 WHEN runtime_status='PROVIDER_PERMISSION_DENIED' THEN 'PERMISSION_DENIED'
 WHEN runtime_status='PROVIDER_RATE_LIMITED' THEN 'RATE_LIMITED'
 WHEN runtime_status IN ('READY','PROVIDER_BLOCKED','ROTATION_BLOCKED_CAPACITY') THEN 'ACTIVE'
 ELSE provider_state END
WHERE provider_state='UNKNOWN';
UPDATE accounts SET provider_state_at=COALESCE(provider_state_at,runtime_status_at),provider_checked_at=COALESCE(provider_checked_at,runtime_status_at);
