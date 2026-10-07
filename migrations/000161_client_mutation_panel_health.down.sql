-- Stop client execution before a binary downgrade; older workers do not enforce panel quarantine.
DROP TABLE client_mutation_panel_health;
ALTER TABLE client_mutation_execution_gate
 DROP COLUMN last_failure_code,
 DROP COLUMN last_failure_at,
 DROP COLUMN last_failure_panel_id,
 DROP COLUMN last_failure_job_id;
