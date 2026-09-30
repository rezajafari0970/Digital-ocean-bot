ALTER TABLE accounts
  ADD COLUMN IF NOT EXISTS console_email TEXT,
  ADD COLUMN IF NOT EXISTS console_password_secret_ref TEXT,
  ADD COLUMN IF NOT EXISTS console_capacity_status TEXT NOT NULL DEFAULT 'unconfigured',
  ADD COLUMN IF NOT EXISTS console_capacity_checked_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS console_capacity_detail TEXT;
