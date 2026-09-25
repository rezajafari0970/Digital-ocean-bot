-- At most one live lifecycle row may own a provider droplet per account.
CREATE UNIQUE INDEX IF NOT EXISTS droplets_live_provider_uidx
ON droplets(account_id,provider_resource_id)
WHERE provider_resource_id IS NOT NULL AND state <> 'DELETED';

-- At most one replacement may be in flight for one expiring droplet.
CREATE UNIQUE INDEX IF NOT EXISTS droplets_replacement_deployment_uidx
ON droplets(replacement_deployment_id)
WHERE replacement_deployment_id IS NOT NULL AND state <> 'DELETED';
