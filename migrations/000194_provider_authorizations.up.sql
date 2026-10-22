CREATE TABLE provider_authorizations (
 id uuid PRIMARY KEY,
 owner_id text NOT NULL,
 provider text NOT NULL,
 proxy_id text NOT NULL DEFAULT '',
 request_hash text NOT NULL,
 state text NOT NULL CHECK(state IN ('DISPATCHED','PENDING','READY','VERIFIED','USED','FAILED','UNKNOWN','EXPIRED','CANCELLED')),
 ciphertext bytea,
 nonce bytea,
 key_version integer,
 external_id text NOT NULL DEFAULT '',
 account_id uuid,
 last_error text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL DEFAULT now()+interval '30 minutes',
 next_check_at timestamptz NOT NULL DEFAULT now(),
 CHECK ((ciphertext IS NULL AND nonce IS NULL AND key_version IS NULL) OR
        (ciphertext IS NOT NULL AND nonce IS NOT NULL AND key_version IS NOT NULL AND key_version>0))
);
CREATE INDEX provider_authorizations_owner ON provider_authorizations(owner_id,created_at DESC);
CREATE INDEX provider_authorizations_expiry ON provider_authorizations(expires_at) WHERE ciphertext IS NOT NULL;
