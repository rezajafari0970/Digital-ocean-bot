CREATE TABLE IF NOT EXISTS server_runtime_snapshots (
  panel_id uuid PRIMARY KEY REFERENCES panel_instances(id) ON DELETE CASCADE,
  checked_at timestamptz NOT NULL DEFAULT now(),
  xui_active boolean NOT NULL DEFAULT false,
  xui_restarted boolean NOT NULL DEFAULT false,
  memory_available_mb bigint NOT NULL DEFAULT 0,
  disk_free_mb bigint NOT NULL DEFAULT 0,
  load_1m double precision NOT NULL DEFAULT 0,
  package_lock_busy boolean NOT NULL DEFAULT false,
  reboot_required boolean NOT NULL DEFAULT false,
  db_present boolean NOT NULL DEFAULT false,
  db_quick_check text NOT NULL DEFAULT '',
  last_error text NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS server_runtime_snapshots_checked_idx ON server_runtime_snapshots(checked_at);
