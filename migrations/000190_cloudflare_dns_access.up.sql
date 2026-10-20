ALTER TABLE cloudflare_zones ADD COLUMN dns_checked_at timestamptz;
ALTER TABLE cloudflare_zones ADD COLUMN dns_error_code text NOT NULL DEFAULT '';
ALTER TABLE cloudflare_zones ADD COLUMN dns_next_attempt_at timestamptz NOT NULL DEFAULT now();
