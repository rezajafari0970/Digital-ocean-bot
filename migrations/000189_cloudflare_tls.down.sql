DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM cloudflare_records WHERE state<>'DELETED') OR EXISTS(SELECT 1 FROM cloudflare_accounts) THEN
  RAISE EXCEPTION 'cloudflare_records_must_be_cleaned_before_downgrade';
 END IF;
END $$;
ALTER TABLE output_config_snapshots DROP COLUMN cloudflare_record_id;
DROP TABLE cloudflare_certificate_history;
DROP TABLE cloudflare_ip_measurements,cloudflare_ip_refresh,cloudflare_ip_ranges;
DROP TABLE cloudflare_api_budget,cloudflare_records,cloudflare_schedules,cloudflare_certificates,cloudflare_zones,cloudflare_secrets,cloudflare_accounts;
