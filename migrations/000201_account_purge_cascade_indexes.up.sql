-- Index every previously uncovered account-purge cascade edge.
-- Existing online indexes are reused on upgrade; no customer data is changed.
CREATE INDEX IF NOT EXISTS purge_server_readiness_issues_readiness_id_idx ON server_readiness_issues (readiness_id);
CREATE INDEX IF NOT EXISTS purge_reality_transport_apply_jobs_rollback_of_idx ON reality_transport_apply_jobs (rollback_of);
CREATE INDEX IF NOT EXISTS purge_config_driver_clients_profile_id_panel_id_idx ON config_driver_clients (profile_id,panel_id);
CREATE INDEX IF NOT EXISTS purge_reality_transport_apply_jobs_panel_id_idx ON reality_transport_apply_jobs (panel_id);
CREATE INDEX IF NOT EXISTS purge_cloudflare_schedules_panel_id_idx ON cloudflare_schedules (panel_id);
CREATE INDEX IF NOT EXISTS purge_deployments_profile_id_idx ON deployments (profile_id);
CREATE INDEX IF NOT EXISTS purge_schedules_profile_id_idx ON schedules (profile_id);
CREATE INDEX IF NOT EXISTS purge_client_mutation_execution_gate_panel_id_idx ON client_mutation_execution_gate (panel_id);
CREATE INDEX IF NOT EXISTS purge_bulk_client_execution_gate_panel_id_idx ON bulk_client_execution_gate (panel_id);
CREATE INDEX IF NOT EXISTS purge_bulk_user_shrink_gate_panel_id_idx ON bulk_user_shrink_gate (panel_id);
DO $$ BEGIN
 IF to_regclass('purge_account_capacity_events_snapshot_id_idx') IS NULL THEN
  CREATE INDEX purge_account_capacity_events_snapshot_id_idx ON account_capacity_events(snapshot_id);
 END IF;
END $$;

DO $$ DECLARE invalid_count integer; BEGIN
 SELECT count(*) INTO invalid_count FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace LEFT JOIN pg_index i ON i.indexrelid=c.oid
 WHERE n.nspname=current_schema() AND c.relname LIKE 'purge_%_idx' AND (i.indisvalid IS DISTINCT FROM true OR i.indisready IS DISTINCT FROM true);
 IF invalid_count<>0 THEN RAISE EXCEPTION 'Purge indexes require online repair before migration'; END IF;
END $$;
