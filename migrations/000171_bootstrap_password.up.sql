ALTER TABLE admin_users ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE admin_users ADD COLUMN bootstrap_token_hash BYTEA;
