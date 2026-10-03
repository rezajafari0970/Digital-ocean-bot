ALTER TABLE rolling_reboot_jobs ADD COLUMN IF NOT EXISTS boot_id_before text NOT NULL DEFAULT '';
