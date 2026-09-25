CREATE UNIQUE INDEX IF NOT EXISTS resources_provider_identity_uidx ON resources(account_id,provider,type,provider_resource_id);
