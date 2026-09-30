ALTER TABLE accounts
  DROP COLUMN IF EXISTS console_capacity_detail,
  DROP COLUMN IF EXISTS console_capacity_checked_at,
  DROP COLUMN IF EXISTS console_capacity_status,
  DROP COLUMN IF EXISTS console_password_secret_ref,
  DROP COLUMN IF EXISTS console_email;
