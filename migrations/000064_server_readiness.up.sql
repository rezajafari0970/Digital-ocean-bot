CREATE TABLE IF NOT EXISTS server_readiness_snapshots (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 run_id uuid NOT NULL REFERENCES provision_runs(id) ON DELETE CASCADE,
 account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 droplet_id uuid NOT NULL REFERENCES droplets(id) ON DELETE CASCADE,
 status text NOT NULL CHECK(status IN ('READY','DEGRADED','BLOCKED')),
 os_id text NOT NULL DEFAULT '',
 os_version text NOT NULL DEFAULT '',
 architecture text NOT NULL DEFAULT '',
 cpu_count integer NOT NULL DEFAULT 0,
 memory_mb bigint NOT NULL DEFAULT 0,
 disk_free_mb bigint NOT NULL DEFAULT 0,
 is_root boolean NOT NULL DEFAULT false,
 package_manager text NOT NULL DEFAULT '',
 package_health text NOT NULL DEFAULT 'unknown',
 dns_ok boolean NOT NULL DEFAULT false,
 outbound_https_ok boolean NOT NULL DEFAULT false,
 time_sync text NOT NULL DEFAULT 'unknown',
 reboot_required boolean NOT NULL DEFAULT false,
 checks jsonb NOT NULL DEFAULT '{}'::jsonb,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS server_readiness_run_idx ON server_readiness_snapshots(run_id,created_at DESC);
CREATE INDEX IF NOT EXISTS server_readiness_status_idx ON server_readiness_snapshots(status,created_at DESC);
