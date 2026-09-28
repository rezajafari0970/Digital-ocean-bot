CREATE TABLE panel_ad_proxies (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 panel_id uuid NOT NULL REFERENCES panel_instances(id) ON DELETE CASCADE,
 proxy_id uuid NOT NULL REFERENCES proxies(id) ON DELETE CASCADE,
 outbound_tag text NOT NULL,
 priority integer NOT NULL DEFAULT 100 CHECK(priority >= 0),
 enabled boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(panel_id,proxy_id),
 UNIQUE(panel_id,outbound_tag)
);
CREATE INDEX panel_ad_proxies_active_idx
 ON panel_ad_proxies(panel_id,priority,id)
 WHERE enabled=true;
