DROP INDEX IF EXISTS droplets_backfill_pending_idx;
ALTER TABLE droplets DROP COLUMN IF EXISTS backfill_required;
