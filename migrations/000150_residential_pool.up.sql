-- Preserve displayed spelling; refuse ambiguous duplicate names, including concurrent writes.
CREATE UNIQUE INDEX residential_name_unique ON residential_proxies(lower(btrim(name)));
CREATE TABLE residential_imports (
 request_id uuid PRIMARY KEY, request_hash text NOT NULL, response jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE residential_proxies ADD COLUMN success_ewma double precision NOT NULL DEFAULT 0,
 ADD COLUMN latency_ewma_ms double precision NOT NULL DEFAULT 0,
 ADD COLUMN check_count bigint NOT NULL DEFAULT 0;
ALTER TABLE panel_routing_state ADD COLUMN pool_enabled boolean NOT NULL DEFAULT false;
