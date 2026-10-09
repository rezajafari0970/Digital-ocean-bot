-- Endpoint and encrypted credential writes share a monotonically increasing identity.
ALTER TABLE residential_proxies ADD COLUMN admission_version bigint NOT NULL DEFAULT 1;
CREATE FUNCTION residential_admission_endpoint_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(NEW.type,NEW.host,NEW.port,NEW.username,NEW.enabled,NEW.secret_ref,NEW.outbound_tag,NEW.priority)
 IS DISTINCT FROM ROW(OLD.type,OLD.host,OLD.port,OLD.username,OLD.enabled,OLD.secret_ref,OLD.outbound_tag,OLD.priority) THEN
  NEW.admission_version := OLD.admission_version+1;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER residential_admission_endpoint_version BEFORE UPDATE ON residential_proxies FOR EACH ROW EXECUTE FUNCTION residential_admission_endpoint_version();
CREATE FUNCTION residential_admission_secret_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' AND NEW.residential_id IS DISTINCT FROM OLD.residential_id THEN
  UPDATE residential_proxies SET admission_version=admission_version+1 WHERE proxy_id=OLD.residential_id;
 END IF;
 UPDATE residential_proxies SET admission_version=admission_version+1 WHERE proxy_id=COALESCE(NEW.residential_id,OLD.residential_id);
 RETURN NULL;
END $$;
CREATE TRIGGER residential_admission_secret_version AFTER INSERT OR UPDATE OR DELETE ON residential_proxy_secrets FOR EACH ROW EXECUTE FUNCTION residential_admission_secret_version();
CREATE TABLE residential_admission_evidence(
 id uuid PRIMARY KEY,
 panel_id uuid NOT NULL REFERENCES panel_instances(id) ON DELETE CASCADE,
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 evidence jsonb NOT NULL CHECK(jsonb_typeof(evidence)='object')
);
CREATE INDEX residential_admission_evidence_panel ON residential_admission_evidence(panel_id,recorded_at DESC);
-- Evidence content is append-only; inventory deletion may remove its history.
CREATE FUNCTION residential_admission_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'admission evidence is immutable'; END $$;
CREATE TRIGGER residential_admission_immutable BEFORE UPDATE ON residential_admission_evidence FOR EACH ROW EXECUTE FUNCTION residential_admission_immutable();
ALTER TABLE residential_performance_operations ADD COLUMN admission_evidence_id uuid UNIQUE;
