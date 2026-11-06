-- Long browser/authorization sessions pin configuration, not live observations.
-- Enforce the pin for every SQL writer, including older clients and deletes.
CREATE FUNCTION guard_pinned_proxy_configuration() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' AND
    (to_jsonb(NEW)-ARRAY['status','exit_ip','country','country_code','asn','failure_count','consecutive_successes',
      'last_checked_at','last_success_at','latency_ms','health_error','updated_at'])
    IS NOT DISTINCT FROM
    (to_jsonb(OLD)-ARRAY['status','exit_ip','country','country_code','asn','failure_count','consecutive_successes',
      'last_checked_at','last_success_at','latency_ms','health_error','updated_at']) THEN
   RETURN NEW;
 END IF;
 IF NOT pg_try_advisory_xact_lock(hashtextextended('proxy-config:'||OLD.id::text,0)) THEN
   RAISE EXCEPTION 'proxy configuration is pinned by an active connection'
     USING ERRCODE='55P03';
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER proxy_configuration_pin BEFORE UPDATE OR DELETE ON proxies
 FOR EACH ROW EXECUTE FUNCTION guard_pinned_proxy_configuration();
