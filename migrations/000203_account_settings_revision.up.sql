-- Only user configuration writes advance this counter; worker observations do not.
ALTER TABLE accounts ADD COLUMN settings_revision bigint NOT NULL DEFAULT 1;
ALTER TABLE accounts ADD CONSTRAINT accounts_settings_revision_positive CHECK (settings_revision > 0);

ALTER TABLE proxies ADD COLUMN settings_revision bigint NOT NULL DEFAULT 1 CHECK(settings_revision>0);
