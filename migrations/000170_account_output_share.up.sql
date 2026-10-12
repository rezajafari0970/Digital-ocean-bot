-- Separate capabilities keep scoped tokens invisible to older global-only readers.
CREATE TABLE account_output_share_tokens (
 token TEXT PRIMARY KEY,
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 route_class TEXT NOT NULL CHECK (route_class IN ('DIRECT','RESIDENTIAL')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 revoked_at TIMESTAMPTZ
);
CREATE INDEX account_output_share_tokens_account_idx ON account_output_share_tokens(account_id);
