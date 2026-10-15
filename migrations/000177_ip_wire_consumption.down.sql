-- Never discard a consumed entitlement or evidence to enable a replay.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM ip_wire_consumptions) THEN RAISE EXCEPTION 'IP wire obligations prevent downgrade'; END IF;
END $$;
DROP TABLE ip_wire_results;
DROP TABLE ip_wire_consumptions;
