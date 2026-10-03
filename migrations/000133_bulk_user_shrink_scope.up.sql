ALTER TABLE bulk_user_shrink_gate
  ADD COLUMN panel_id uuid REFERENCES panel_instances(id) ON DELETE SET NULL,
  ADD COLUMN inbound_id bigint CHECK(inbound_id IS NULL OR inbound_id > 0),
  ADD CONSTRAINT bulk_user_shrink_gate_scope CHECK(panel_id IS NOT NULL OR inbound_id IS NULL);
