DROP INDEX IF EXISTS bulk_user_ownership_planned_recovery_idx;
ALTER TABLE bulk_user_ownership DROP COLUMN IF EXISTS last_recovery_check_at;
ALTER TABLE bulk_user_ownership DROP COLUMN IF EXISTS recovery_checks;
ALTER TABLE bulk_user_ownership DROP CONSTRAINT bulk_user_ownership_state_check;
UPDATE bulk_user_ownership SET state='DELETED',deleted_at=coalesce(deleted_at,now()) WHERE state='ABORTED';
ALTER TABLE bulk_user_ownership ADD CONSTRAINT bulk_user_ownership_state_check CHECK (state IN ('PLANNED','ACTIVE','DELETE_PENDING','DELETED'));
