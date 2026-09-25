ALTER TABLE accounts ADD COLUMN IF NOT EXISTS browser_challenge_type TEXT NOT NULL DEFAULT 'none';
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS browser_challenge_at TIMESTAMPTZ;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS browser_challenge_detail TEXT;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS password_rotation_requested_at TIMESTAMPTZ;
ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_browser_challenge_type_check;
ALTER TABLE accounts ADD CONSTRAINT accounts_browser_challenge_type_check CHECK(browser_challenge_type IN ('none','captcha','two_factor','reauth'));
