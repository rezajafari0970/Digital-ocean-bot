ALTER TABLE proxies DROP CONSTRAINT IF EXISTS proxies_adapter_check;
ALTER TABLE proxies DROP COLUMN IF EXISTS adapter;
