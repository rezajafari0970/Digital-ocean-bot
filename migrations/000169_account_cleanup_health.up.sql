-- Full FK indexes are required during account cascade deletion. Partial
-- observation indexes cannot serve RI checks for unfinished historical rows.
CREATE INDEX IF NOT EXISTS account_cleanup_panel_sync_idx ON panel_inventory_syncs(panel_id);
CREATE INDEX IF NOT EXISTS account_cleanup_snapshots_idx ON provider_snapshots(account_id);
CREATE INDEX IF NOT EXISTS account_cleanup_lifecycle_idx ON lifecycle_events(account_id);
CREATE INDEX IF NOT EXISTS account_cleanup_provision_droplet_idx ON provision_runs(droplet_id);
CREATE INDEX IF NOT EXISTS account_cleanup_xui_db_idx ON xui_database_deployments(account_id);
CREATE INDEX IF NOT EXISTS account_cleanup_deployments_idx ON deployments(account_id);
CREATE INDEX IF NOT EXISTS account_cleanup_deployment_droplet_idx ON deployments(droplet_id);
CREATE INDEX IF NOT EXISTS account_cleanup_failures_idx ON worker_item_failures(account_id);
CREATE INDEX IF NOT EXISTS account_cleanup_readiness_idx ON server_readiness_snapshots(account_id);
CREATE INDEX IF NOT EXISTS account_cleanup_readiness_droplet_idx ON server_readiness_snapshots(droplet_id);
CREATE INDEX IF NOT EXISTS account_cleanup_installer_run_idx ON installer_runs(provision_run_id);
CREATE INDEX IF NOT EXISTS account_cleanup_xui_panel_idx ON xui_panel_deployments(account_id);
CREATE INDEX IF NOT EXISTS account_cleanup_panels_idx ON panel_instances(account_id);
CREATE INDEX IF NOT EXISTS account_cleanup_reboot_droplet_idx ON rolling_reboot_jobs(droplet_id);
CREATE INDEX IF NOT EXISTS account_cleanup_reboot_account_idx ON rolling_reboot_jobs(account_id);
CREATE INDEX IF NOT EXISTS account_cleanup_generations_idx ON bulk_user_generations(panel_id);
CREATE INDEX IF NOT EXISTS account_cleanup_ownership_job_idx ON bulk_user_ownership(mutation_job_id);
CREATE INDEX IF NOT EXISTS account_cleanup_scope_generation_idx ON bulk_lifecycle_scopes(generation_id);
CREATE INDEX IF NOT EXISTS account_cleanup_targets_idx ON panel_cleanup_targets(panel_id);
CREATE INDEX IF NOT EXISTS account_cleanup_waiting_droplet_idx ON account_rule_application(waiting_droplet_id);
CREATE INDEX IF NOT EXISTS account_cleanup_panel_job_idx ON client_mutation_panel_health(last_job_id);
CREATE INDEX IF NOT EXISTS account_cleanup_relay_endpoint_idx ON panel_relay_endpoints(account_id);
CREATE INDEX IF NOT EXISTS account_cleanup_relay_panel_idx ON panel_relay_assignments(donor_panel_id);
CREATE INDEX IF NOT EXISTS account_cleanup_relay_droplet_idx ON panel_relay_assignments(donor_droplet_id);
CREATE INDEX IF NOT EXISTS account_cleanup_relay_account_idx ON panel_relay_assignments(donor_account_id);

CREATE TABLE account_purge_requests (
 account_id uuid PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
 expected_provider_state text NOT NULL,
 expected_resource_scope text NOT NULL CHECK(length(expected_resource_scope)=64),
 expected_remaining_servers integer NOT NULL CHECK(expected_remaining_servers>=0),
 deletion_requested_at timestamptz NOT NULL,
 acknowledged_at timestamptz NOT NULL DEFAULT now(),
 next_attempt_at timestamptz NOT NULL DEFAULT now(),
 attempts integer NOT NULL DEFAULT 0,
 state text NOT NULL DEFAULT 'QUEUED' CHECK(state IN ('QUEUED','RETRY','NEEDS_REVIEW')),
 last_error text NOT NULL DEFAULT ''
);
CREATE INDEX account_purge_requests_due_idx ON account_purge_requests(next_attempt_at,account_id) WHERE state<>'NEEDS_REVIEW';

-- Display evidence only; never used to loosen pre-mutation freshness.
ALTER TABLE accounts ADD COLUMN provider_inventory_interval_seconds integer NOT NULL DEFAULT 90 CHECK(provider_inventory_interval_seconds BETWEEN 1 AND 300);
