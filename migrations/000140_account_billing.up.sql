CREATE TABLE account_billing_snapshots(
 account_id uuid PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
 data jsonb NOT NULL DEFAULT '{}'::jsonb,
 observed_at timestamptz,
 attempted_at timestamptz NOT NULL DEFAULT now(),
 next_attempt_at timestamptz NOT NULL DEFAULT now(),
 last_error text NOT NULL DEFAULT ''
);
