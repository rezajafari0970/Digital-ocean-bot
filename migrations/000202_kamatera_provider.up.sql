ALTER TABLE provider_account_resources ADD COLUMN operation_id text NOT NULL DEFAULT '';
ALTER TABLE provider_absence_evidence DROP CONSTRAINT provider_absence_evidence_provider_check;
ALTER TABLE provider_absence_evidence ADD CONSTRAINT provider_absence_evidence_provider_check CHECK(provider IN ('aws','ovh','kamatera'));
ALTER TABLE provider_account_resources ADD COLUMN intent jsonb NOT NULL DEFAULT '{}';
ALTER TABLE provider_account_resources ADD COLUMN delete_attempted_at timestamptz;
CREATE UNIQUE INDEX provider_kamatera_resource_binding_idx ON provider_account_resources(provider,resource_id) WHERE provider='kamatera' AND resource_id<>'';
