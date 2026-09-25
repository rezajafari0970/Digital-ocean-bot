ALTER TABLE accounts
  ADD COLUMN IF NOT EXISTS login_email TEXT,
  ADD COLUMN IF NOT EXISTS login_password_secret_ref TEXT,
  ADD COLUMN IF NOT EXISTS password_rotation_status TEXT NOT NULL DEFAULT 'disabled',
  ADD COLUMN IF NOT EXISTS password_rotated_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS password_rotation_detail TEXT;

CREATE INDEX IF NOT EXISTS accounts_password_rotation_idx
  ON accounts(password_rotation_status, updated_at)
  WHERE password_rotation_status NOT IN ('disabled','completed');
