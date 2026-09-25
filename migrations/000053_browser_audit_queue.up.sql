CREATE TABLE IF NOT EXISTS browser_audit_queue (
 account_id uuid PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
 requested_at timestamptz NOT NULL DEFAULT now(),
 not_before timestamptz NOT NULL DEFAULT now(),
 reason text NOT NULL DEFAULT 'geo_changed',
 attempts integer NOT NULL DEFAULT 0,
 last_error text
);
