-- Health observations govern Output immediately; they do not change routing
-- intent. Keep a known enabled residential endpoint as the protected egress
-- even when its health probe fails. Transport failure cannot become direct.
DROP TRIGGER residential_route_update ON residential_proxies;
CREATE TRIGGER residential_route_update AFTER UPDATE ON residential_proxies FOR EACH ROW WHEN
 ((OLD.type,OLD.host,OLD.port,OLD.username,OLD.secret_ref,OLD.priority,OLD.enabled)
 IS DISTINCT FROM (NEW.type,NEW.host,NEW.port,NEW.username,NEW.secret_ref,NEW.priority,NEW.enabled))
 EXECUTE FUNCTION bump_residential_routing_revision();
