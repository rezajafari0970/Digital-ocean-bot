-- Restore health-triggered fleet invalidation for rollback.
DROP TRIGGER residential_route_update ON residential_proxies;
CREATE TRIGGER residential_route_update AFTER UPDATE ON residential_proxies FOR EACH ROW WHEN
 ((OLD.type,OLD.host,OLD.port,OLD.username,OLD.secret_ref,OLD.priority,OLD.enabled,OLD.status)
 IS DISTINCT FROM (NEW.type,NEW.host,NEW.port,NEW.username,NEW.secret_ref,NEW.priority,NEW.enabled,NEW.status))
 EXECUTE FUNCTION bump_residential_routing_revision();
