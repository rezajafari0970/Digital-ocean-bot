-- Never discard unresolved reconciliation diagnostics during a downgrade.
DO $$BEGIN
 IF EXISTS(SELECT 1 FROM worker_recovery_checkpoints WHERE reconcile_error<>'') THEN
  RAISE EXCEPTION 'unresolved recovery reconciliation prevents downgrade';
 END IF;
 IF EXISTS(SELECT 1 FROM deployment_step_attempts a JOIN deployments d ON d.id=a.deployment_id
 WHERE a.result_snapshot IS NOT NULL AND a.step=d.current_step
 AND a.generation=CASE WHEN a.step IN ('database','panel') THEN d.postinstall_generation ELSE 0 END
 AND d.state NOT IN ('READY','FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK','PANEL_COMPLETE','WAITING_INSTALLER')) THEN
  RAISE EXCEPTION 'unapplied workflow result prevents downgrade';
 END IF;
END$$;
ALTER TABLE deployment_step_attempts DROP COLUMN result_snapshot;
DROP INDEX worker_recovery_checkpoints_reconcile_idx;
DROP INDEX worker_recovery_checkpoints_account_idx;
ALTER TABLE worker_recovery_checkpoints DROP COLUMN reconcile_after,DROP COLUMN reconcile_error;
