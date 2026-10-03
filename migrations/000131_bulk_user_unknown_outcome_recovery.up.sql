ALTER TABLE bulk_user_ownership
  DROP CONSTRAINT bulk_user_ownership_state_check;

ALTER TABLE bulk_user_ownership
  ADD CONSTRAINT bulk_user_ownership_state_check
  CHECK (state IN ('PLANNED','ACTIVE','DELETE_PENDING','DELETED','ABORTED'));

ALTER TABLE bulk_user_ownership
  ADD COLUMN recovery_checks integer NOT NULL DEFAULT 0 CHECK (recovery_checks >= 0),
  ADD COLUMN last_recovery_check_at timestamptz;

CREATE INDEX bulk_user_ownership_planned_recovery_idx
ON bulk_user_ownership(generation_id,created_at,recovery_checks)
WHERE state='PLANNED';
