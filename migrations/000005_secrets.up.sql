CREATE TABLE secrets (
 id TEXT NOT NULL,
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 kind TEXT NOT NULL,
 ciphertext BYTEA NOT NULL,
 nonce BYTEA NOT NULL,
 key_version INTEGER NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY (account_id, id)
);
CREATE INDEX secrets_account_kind_idx ON secrets(account_id, kind);
