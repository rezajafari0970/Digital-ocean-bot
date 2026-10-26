DO $$ BEGIN IF EXISTS(SELECT 1 FROM accounts WHERE provider='kamatera') THEN RAISE EXCEPTION 'Kamatera account cleanup required before downgrade'; END IF; END $$;
ALTER TABLE provider_absence_evidence DROP CONSTRAINT provider_absence_evidence_provider_check;
ALTER TABLE provider_absence_evidence ADD CONSTRAINT provider_absence_evidence_provider_check CHECK(provider IN ('aws','ovh'));
ALTER TABLE provider_account_resources DROP COLUMN operation_id;
DROP INDEX IF EXISTS provider_kamatera_resource_binding_idx;
ALTER TABLE provider_account_resources DROP COLUMN intent;
ALTER TABLE provider_account_resources DROP COLUMN delete_attempted_at;
