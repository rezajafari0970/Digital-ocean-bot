CREATE TABLE provider_absence_evidence (
 account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 provider text NOT NULL CHECK (provider IN ('aws','ovh')),
 server_id text NOT NULL,
 first_observed_at timestamptz NOT NULL DEFAULT now(),
 last_observed_at timestamptz NOT NULL DEFAULT now(),
 observations integer NOT NULL DEFAULT 1 CHECK(observations > 0),
 PRIMARY KEY(account_id,provider,server_id)
);
