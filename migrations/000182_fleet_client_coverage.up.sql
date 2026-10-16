-- Scope coverage is not execution concurrency. Zero means every eligible
-- listener; the serialized executor, per-scope budgets and gates still apply.
ALTER TABLE bulk_lifecycle_control DROP CONSTRAINT bulk_lifecycle_control_max_active_scopes_check;
ALTER TABLE bulk_lifecycle_control ADD CONSTRAINT bulk_lifecycle_control_max_active_scopes_check CHECK(max_active_scopes BETWEEN 0 AND 256);
ALTER TABLE bulk_lifecycle_control ALTER COLUMN max_active_scopes SET DEFAULT 0;
-- Upgrade only the historical default for an already authorized global lane.
-- Explicit custom limits and scoped/disabled execution gates remain unchanged.
UPDATE bulk_lifecycle_control c SET max_active_scopes=0,updated_at=now()
FROM client_mutation_execution_gate g WHERE c.singleton AND g.singleton
AND c.enabled AND c.auto_enroll AND c.max_active_scopes=64
AND g.enabled AND NOT g.kill_switch AND g.panel_id IS NULL AND g.inbound_id IS NULL AND g.concurrency=1;
