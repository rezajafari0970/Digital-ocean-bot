ALTER TABLE config_driver_profiles ADD COLUMN class_settings jsonb NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(class_settings)='object');
ALTER TABLE config_driver_listeners ADD COLUMN stage_started_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE config_driver_profiles ADD COLUMN last_scheduled_at timestamptz;
ALTER TABLE config_driver_clients ADD COLUMN expires_at timestamptz;
ALTER TABLE config_driver_clients ADD COLUMN publish boolean NOT NULL DEFAULT true;
CREATE TABLE config_driver_class_proofs (
 profile_id uuid NOT NULL,panel_id uuid NOT NULL,route_class text NOT NULL CHECK(route_class IN('DIRECT','RESIDENTIAL')),
 verified_at timestamptz,error_code text NOT NULL DEFAULT '',
 PRIMARY KEY(profile_id,panel_id,route_class),FOREIGN KEY(profile_id,panel_id) REFERENCES config_driver_listeners(profile_id,panel_id) ON DELETE CASCADE);
INSERT INTO config_driver_class_proofs(profile_id,panel_id,route_class,verified_at)
 SELECT l.profile_id,l.panel_id,c,l.verified_at FROM config_driver_listeners l JOIN config_driver_profiles p ON p.id=l.profile_id CROSS JOIN LATERAL unnest(p.classes)c WHERE l.state='VERIFIED';
CREATE TABLE config_driver_sni_selections (
 profile_id uuid NOT NULL,panel_id uuid NOT NULL,revision bigint NOT NULL,sni text NOT NULL,
 verified_at timestamptz NOT NULL,latency_ms bigint NOT NULL,
 PRIMARY KEY(profile_id,panel_id),FOREIGN KEY(profile_id,panel_id) REFERENCES config_driver_listeners(profile_id,panel_id) ON DELETE CASCADE);
ALTER TABLE config_driver_listeners ADD COLUMN lease_until timestamptz, ADD COLUMN claim_token uuid;
CREATE INDEX config_driver_listener_active_leases ON config_driver_listeners(panel_id,lease_until) WHERE claim_token IS NOT NULL;
ALTER TABLE config_driver_listeners ADD COLUMN needs_reconcile boolean NOT NULL DEFAULT false;
