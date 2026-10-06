-- A failed poll must not erase the last verified admission decision. In
-- particular, age or an operator disable request is not proof that nft rules
-- have actually been removed on an unreachable node.
ALTER TABLE server_protection_nodes
 ADD COLUMN verified_status jsonb NOT NULL DEFAULT '{}'::jsonb,
 ADD COLUMN verified_at timestamptz;
UPDATE server_protection_nodes SET verified_status=status,verified_at=checked_at
 WHERE state IN ('APPLIED','DISABLED') AND applied_revision=desired_revision
 AND COALESCE(last_error,'')='';
CREATE INDEX IF NOT EXISTS panel_inventory_syncs_recent_idx
 ON panel_inventory_syncs(panel_id,finished_at DESC,id DESC) WHERE finished_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS panel_inventory_syncs_retention_idx
 ON panel_inventory_syncs(finished_at) WHERE state='COMPLETED';
