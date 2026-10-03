ALTER TABLE bulk_user_shrink_gate DROP CONSTRAINT IF EXISTS bulk_user_shrink_gate_scope;
ALTER TABLE bulk_user_shrink_gate DROP COLUMN IF EXISTS inbound_id;
ALTER TABLE bulk_user_shrink_gate DROP COLUMN IF EXISTS panel_id;
