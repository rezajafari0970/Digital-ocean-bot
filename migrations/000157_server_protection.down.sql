-- Remote protection must be disabled and verified before downgrade.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM server_protection_control WHERE enabled)
 OR EXISTS(SELECT 1 FROM server_protection_nodes n JOIN panel_instances p ON p.id=n.panel_id JOIN droplets d ON d.id=p.droplet_id WHERE d.state<>'DELETED' AND (n.desired_enabled OR n.state<>'DISABLED' OR n.applied_revision<>n.desired_revision))
 THEN RAISE EXCEPTION 'Disable server protection and verify remote cleanup before downgrade'; END IF;
END $$;
DROP TABLE server_protection_requests;
DROP TABLE server_protection_nodes;
DROP TABLE server_protection_control;
