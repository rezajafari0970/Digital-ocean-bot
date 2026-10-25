-- Match the completed-only latest-witness probe even before ANALYZE has
-- learned a burst's distribution. The broader recent index must still serve
-- non-completed inventory readers. No observation or failure is rewritten.
CREATE INDEX IF NOT EXISTS panel_inventory_syncs_completed_recent_idx
 ON panel_inventory_syncs(panel_id,finished_at DESC)
 WHERE state='COMPLETED' AND finished_at IS NOT NULL;
