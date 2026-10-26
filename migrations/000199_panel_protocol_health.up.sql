-- UDP evidence belongs to a receiving node and an exact native outbound.
-- Keep TCP monitoring/selection independent. No credential material is stored.
CREATE TABLE panel_protocol_health (
 panel_id uuid NOT NULL REFERENCES panel_instances(id) ON DELETE CASCADE,
 proxy_id text NOT NULL,
 origin_hash text NOT NULL CHECK (origin_hash ~ '^[0-9a-f]{64}$'),
 transport_hash text NOT NULL CHECK (transport_hash ~ '^[0-9a-f]{64}$'),
 dns_ok boolean NOT NULL,
 quic_ok boolean NOT NULL,
 code text NOT NULL CHECK (code IN ('ok','datagram_unverified')),
 checked_at timestamptz NOT NULL,
 PRIMARY KEY(panel_id,proxy_id),
 CHECK ((code='ok')=(dns_ok AND quic_ok))
);
CREATE TABLE panel_protocol_probe_schedule (
 panel_id uuid PRIMARY KEY REFERENCES panel_instances(id) ON DELETE CASCADE,
 next_probe_at timestamptz NOT NULL DEFAULT now(),
 lease_token text NOT NULL DEFAULT '',
 lease_until timestamptz NOT NULL DEFAULT '-infinity',
 last_error text NOT NULL DEFAULT '' CHECK(last_error IN ('','probe_unavailable'))
);
CREATE INDEX panel_protocol_probe_due ON panel_protocol_probe_schedule(next_probe_at);
