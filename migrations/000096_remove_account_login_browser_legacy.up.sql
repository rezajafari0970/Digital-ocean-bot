DELETE FROM secrets
WHERE kind = 'digitalocean_login_password';

DROP TABLE IF EXISTS browser_audit_queue;
DROP TABLE IF EXISTS account_browser_identities;

ALTER TABLE accounts
    DROP COLUMN IF EXISTS login_email,
    DROP COLUMN IF EXISTS login_password_secret_ref,
    DROP COLUMN IF EXISTS password_rotation_status,
    DROP COLUMN IF EXISTS password_rotation_detail,
    DROP COLUMN IF EXISTS assigned_browser,
    DROP COLUMN IF EXISTS browser_assigned_at,
    DROP COLUMN IF EXISTS browser_login_status,
    DROP COLUMN IF EXISTS browser_login_detail,
    DROP COLUMN IF EXISTS browser_login_checked_at,
    DROP COLUMN IF EXISTS browser_challenge_type,
    DROP COLUMN IF EXISTS browser_challenge_at,
    DROP COLUMN IF EXISTS browser_challenge_detail,
    DROP COLUMN IF EXISTS password_rotation_requested_at,
    DROP COLUMN IF EXISTS password_rotated_at;
