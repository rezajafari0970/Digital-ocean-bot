DROP INDEX IF EXISTS account_browser_identities_runtime_idx;

ALTER TABLE accounts
DROP CONSTRAINT IF EXISTS accounts_assigned_browser_check;

ALTER TABLE accounts
DROP COLUMN IF EXISTS browser_assigned_at;

ALTER TABLE accounts
DROP COLUMN IF EXISTS assigned_browser;

ALTER TABLE account_browser_identities
DROP COLUMN IF EXISTS runtime_engine;

ALTER TABLE account_browser_identities
DROP COLUMN IF EXISTS runtime_version;

ALTER TABLE account_browser_identities
DROP COLUMN IF EXISTS audit_error;

ALTER TABLE account_browser_identities
DROP COLUMN IF EXISTS audit_status;
