ALTER TABLE worker_recovery_checkpoints
 ADD COLUMN reconcile_after timestamptz NOT NULL DEFAULT now(),
 ADD COLUMN reconcile_error text NOT NULL DEFAULT '';
CREATE INDEX worker_recovery_checkpoints_reconcile_idx ON worker_recovery_checkpoints(reconcile_after,created_at);
CREATE INDEX worker_recovery_checkpoints_account_idx ON worker_recovery_checkpoints(account_id);
ALTER TABLE deployment_step_attempts ADD COLUMN result_snapshot jsonb;
