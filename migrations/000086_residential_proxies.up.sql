CREATE TABLE residential_proxies (
 proxy_id uuid PRIMARY KEY REFERENCES proxies(id) ON DELETE CASCADE,
 outbound_tag text NOT NULL UNIQUE,
 priority integer NOT NULL DEFAULT 100 CHECK(priority >= 0),
 enabled boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX residential_proxies_active_priority_idx ON residential_proxies(priority,proxy_id) WHERE enabled=true;
