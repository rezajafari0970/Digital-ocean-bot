ALTER TABLE reality_config_profiles ADD COLUMN creation_interval_seconds integer NOT NULL DEFAULT 0 CHECK (creation_interval_seconds BETWEEN 0 AND 86400);
CREATE TABLE config_creation_schedule (
 panel_id uuid NOT NULL REFERENCES panel_instances(id) ON DELETE CASCADE,
 inbound_id bigint NOT NULL CHECK (inbound_id>0),
 route_class text NOT NULL REFERENCES reality_config_profiles(route_class) ON DELETE CASCADE,
 last_planned_at timestamptz,
 PRIMARY KEY(panel_id,inbound_id,route_class)
);
-- Seed from durable identities, including deleted ones: enabling an interval
-- or restarting the worker must not reset the clock and create a burst.
INSERT INTO config_creation_schedule(panel_id,inbound_id,route_class,last_planned_at)
SELECT g.panel_id,g.inbound_id,o.route_class,max(o.created_at)
FROM bulk_user_ownership o JOIN bulk_user_generations g ON g.id=o.generation_id
JOIN reality_config_profiles p ON p.route_class=o.route_class
GROUP BY g.panel_id,g.inbound_id,o.route_class;
