ALTER TABLE accounts ADD COLUMN IF NOT EXISTS browser_login_status TEXT NOT NULL DEFAULT 'not_tested';
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS browser_login_detail TEXT;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS browser_login_checked_at TIMESTAMPTZ;
