ALTER TABLE reality_target_selections
DROP COLUMN IF EXISTS scanned_at,
DROP COLUMN IF EXISTS latency_ms,
DROP COLUMN IF EXISTS cert_chain_valid,
DROP COLUMN IF EXISTS cert_valid,
DROP COLUMN IF EXISTS curve_id,
DROP COLUMN IF EXISTS alpn,
DROP COLUMN IF EXISTS tls_version,
DROP COLUMN IF EXISTS server_names;
