-- Rollback cannot erase unresolved resources or billing evidence.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM ip_mutation_intents) OR EXISTS(SELECT 1 FROM ip_allocations)
 OR EXISTS(SELECT 1 FROM ip_publication_leases) OR EXISTS(SELECT 1 FROM ip_native_delete_fences) THEN
  RAISE EXCEPTION 'IP lifecycle ledger is nonempty; reconcile and retain it before rollback';
 END IF;
END $$;
DROP TRIGGER ip_account_purge_guard ON accounts;
DROP FUNCTION ip_guard_account_purge();
DROP FUNCTION ip_account_obligations(uuid,text);
DROP TABLE ip_cleanup_credentials;
DROP FUNCTION ip_guard_cleanup_credential();
DROP TABLE ip_resource_incidents,ip_operation_reservations,ip_funding_reservations;
ALTER TABLE ip_approvals DROP COLUMN funding_scope;
DROP TABLE ip_funding_events,ip_account_funding,ip_execution_evidence,ip_capacity_evidence,ip_lifecycle_inventory_receipts;
DROP TABLE ip_network_footprints,ip_native_delete_fences,ip_publication_leases,ip_endpoint_proofs,ip_endpoint_history,ip_endpoints,ip_allocations,ip_mutation_lanes,ip_mutation_receipts,ip_mutation_intents,ip_approvals,ip_plans,ip_policy_events,ip_policies;
DROP FUNCTION ip_reject_ledger_change();

DROP FUNCTION ip_freeze_intent();
DROP FUNCTION ip_freeze_approval();

DROP FUNCTION ip_invalidate_network_proofs();

DROP FUNCTION ip_freeze_evidence();
