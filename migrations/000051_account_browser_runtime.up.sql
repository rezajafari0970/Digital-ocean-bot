ALTER TABLE accounts
ADD COLUMN IF NOT EXISTS assigned_browser TEXT;

ALTER TABLE accounts
ADD COLUMN IF NOT EXISTS browser_assigned_at TIMESTAMPTZ;

ALTER TABLE account_browser_identities
ADD COLUMN IF NOT EXISTS runtime TEXT NOT NULL DEFAULT 'chromium';

ALTER TABLE account_browser_identities
ADD COLUMN IF NOT EXISTS audit_status TEXT NOT NULL DEFAULT 'unknown';

ALTER TABLE account_browser_identities
ADD COLUMN IF NOT EXISTS audit_error TEXT;

ALTER TABLE account_browser_identities
ADD COLUMN IF NOT EXISTS runtime_version TEXT;

ALTER TABLE account_browser_identities
ADD COLUMN IF NOT EXISTS runtime_engine TEXT;

-- Remove old one-row-per-account primary key if present.
DO $$
DECLARE
    pk_name text;
BEGIN
    SELECT tc.constraint_name
    INTO pk_name
    FROM information_schema.table_constraints tc
    WHERE tc.table_schema='public'
      AND tc.table_name='account_browser_identities'
      AND tc.constraint_type='PRIMARY KEY'
    LIMIT 1;

    IF pk_name IS NOT NULL THEN
        EXECUTE format(
            'ALTER TABLE account_browser_identities DROP CONSTRAINT %I',
            pk_name
        );
    END IF;
END $$;

-- New identity: one audit observation per Account + real runtime.
ALTER TABLE account_browser_identities
ADD CONSTRAINT account_browser_identities_pkey
PRIMARY KEY(account_id,runtime);

ALTER TABLE accounts
DROP CONSTRAINT IF EXISTS accounts_assigned_browser_check;

ALTER TABLE accounts
ADD CONSTRAINT accounts_assigned_browser_check
CHECK (
    assigned_browser IS NULL OR
    assigned_browser IN (
        'chromium',
        'chrome',
        'firefox',
        'edge',
        'brave',
        'vivaldi',
        'opera'
    )
);

CREATE INDEX IF NOT EXISTS
account_browser_identities_runtime_idx
ON account_browser_identities(runtime,checked_at DESC);
