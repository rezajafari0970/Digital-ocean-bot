ALTER TABLE proxies ADD COLUMN IF NOT EXISTS adapter TEXT NOT NULL DEFAULT 'generic';
ALTER TABLE proxies DROP CONSTRAINT IF EXISTS proxies_adapter_check;
ALTER TABLE proxies ADD CONSTRAINT proxies_adapter_check CHECK(adapter IN ('generic','suffix-session'));
