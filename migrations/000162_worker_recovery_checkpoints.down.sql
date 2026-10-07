DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM worker_recovery_checkpoints) THEN
  RAISE EXCEPTION 'unresolved recovery checkpoints; reconcile with checkpoint-aware worker before downgrade';
 END IF;
END $$;
DROP TABLE worker_recovery_checkpoints;
