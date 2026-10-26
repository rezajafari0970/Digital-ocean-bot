-- Managed relays intentionally deny generic DNS/IP destinations. Their proof
-- is QUIC reachability through the permitted category, not DNS support.
ALTER TABLE panel_protocol_health ADD COLUMN driver text NOT NULL DEFAULT 'socks5'
 CHECK (driver IN ('socks5','managed_tls_xudp'));
ALTER TABLE panel_protocol_health DROP CONSTRAINT panel_protocol_health_check;
ALTER TABLE panel_protocol_health ADD CONSTRAINT panel_protocol_health_result_check
 CHECK ((code='ok')=(quic_ok AND (driver='managed_tls_xudp' OR dns_ok)));
