-- Independent residential endpoints, credentials and health.
DROP TRIGGER residential_proxy_change ON proxies;
DROP TRIGGER residential_secret_change ON secrets;
DROP TRIGGER residential_route_change ON residential_proxies;
ALTER TABLE residential_proxies DROP CONSTRAINT residential_proxies_proxy_id_fkey;
ALTER TABLE residential_proxies ADD COLUMN name text, ADD COLUMN type text,
 ADD COLUMN host text, ADD COLUMN port integer, ADD COLUMN username text,
 ADD COLUMN secret_ref text, ADD COLUMN status text NOT NULL DEFAULT 'unknown',
 ADD COLUMN exit_ip inet, ADD COLUMN country text, ADD COLUMN latency_ms bigint,
 ADD COLUMN last_checked_at timestamptz, ADD COLUMN last_success_at timestamptz,
 ADD COLUMN last_error text NOT NULL DEFAULT '';
UPDATE residential_proxies rp SET name=p.name,type=p.type,host=p.host,port=p.port,
 username=p.username,secret_ref=p.secret_ref FROM proxies p WHERE p.id=rp.proxy_id;
ALTER TABLE residential_proxies ALTER COLUMN name SET NOT NULL, ALTER COLUMN type SET NOT NULL,
 ALTER COLUMN host SET NOT NULL, ALTER COLUMN port SET NOT NULL,
 ADD CHECK(type IN ('http','https','socks5')), ADD CHECK(port BETWEEN 1 AND 65535),
 ADD CHECK(status IN ('unknown','healthy','down'));
CREATE TABLE residential_proxy_secrets(
 residential_id uuid NOT NULL REFERENCES residential_proxies(proxy_id) ON DELETE CASCADE,
 id text NOT NULL, kind text NOT NULL, ciphertext bytea NOT NULL, nonce bytea NOT NULL,
 key_version integer NOT NULL, aad_scope text NOT NULL DEFAULT 'residential' CHECK(aad_scope IN ('proxy','residential')),
 updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(residential_id,id));
-- Preserve authenticated encryption bindings without decrypting migration data.
INSERT INTO residential_proxy_secrets(residential_id,id,kind,ciphertext,nonce,key_version,aad_scope)
 SELECT s.proxy_id,s.id,s.kind,s.ciphertext,s.nonce,s.key_version,'proxy'
 FROM secrets s JOIN residential_proxies rp ON rp.proxy_id=s.proxy_id;
ALTER TABLE panel_routing_state DROP CONSTRAINT panel_routing_state_selected_proxy_id_fkey;
UPDATE panel_routing_state SET selected_proxy_id=NULL WHERE selected_proxy_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM residential_proxies WHERE proxy_id=selected_proxy_id);
ALTER TABLE panel_routing_state ADD FOREIGN KEY(selected_proxy_id) REFERENCES residential_proxies(proxy_id) ON DELETE SET NULL;
DELETE FROM proxies p WHERE EXISTS(SELECT 1 FROM residential_proxies rp WHERE rp.proxy_id=p.id)
 AND NOT EXISTS(SELECT 1 FROM network_profiles WHERE proxy_id=p.id)
 AND NOT EXISTS(SELECT 1 FROM account_proxy_pool WHERE proxy_id=p.id)
 AND NOT EXISTS(SELECT 1 FROM panel_ad_proxies WHERE proxy_id=p.id);
CREATE TRIGGER residential_route_insert_delete AFTER INSERT OR DELETE ON residential_proxies
 FOR EACH STATEMENT EXECUTE FUNCTION bump_residential_routing_revision();
CREATE TRIGGER residential_route_update AFTER UPDATE ON residential_proxies FOR EACH ROW WHEN
 ((OLD.type,OLD.host,OLD.port,OLD.username,OLD.secret_ref,OLD.priority,OLD.enabled,OLD.status)
 IS DISTINCT FROM (NEW.type,NEW.host,NEW.port,NEW.username,NEW.secret_ref,NEW.priority,NEW.enabled,NEW.status))
 EXECUTE FUNCTION bump_residential_routing_revision();
CREATE TRIGGER residential_credential_change AFTER INSERT OR UPDATE OR DELETE ON residential_proxy_secrets
 FOR EACH STATEMENT EXECUTE FUNCTION bump_residential_routing_revision();
UPDATE residential_routing_control SET revision=revision+1 WHERE singleton;
UPDATE panel_routing_state SET next_check_at=now();
