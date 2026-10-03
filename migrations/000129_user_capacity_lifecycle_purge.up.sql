CREATE OR REPLACE FUNCTION purge_user_capacity_snapshots_on_droplet_retire()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.state IN ('RETIRING','DELETING','DELETED')
     AND OLD.state IS DISTINCT FROM NEW.state THEN
    DELETE FROM user_capacity_snapshots u
    USING panel_instances p
    WHERE p.droplet_id=NEW.id AND u.panel_id=p.id;
  END IF;
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS droplets_purge_user_capacity_snapshots ON droplets;
CREATE TRIGGER droplets_purge_user_capacity_snapshots
AFTER UPDATE OF state ON droplets
FOR EACH ROW
WHEN (NEW.state IN ('RETIRING','DELETING','DELETED'))
EXECUTE FUNCTION purge_user_capacity_snapshots_on_droplet_retire();

DELETE FROM user_capacity_snapshots u
WHERE NOT EXISTS (
  SELECT 1
  FROM panel_instances p
  JOIN panel_inbound_inventory i
    ON i.panel_id=p.id AND i.remote_id=u.inbound_id
  JOIN droplets d ON d.id=p.droplet_id
  JOIN accounts a ON a.id=p.account_id
  JOIN deployments dep ON dep.droplet_id=d.id
  WHERE p.id=u.panel_id
    AND p.enabled=true
    AND i.present=true AND i.enabled=true
    AND a.enabled=true AND a.provider_state='ACTIVE'
    AND d.state IN ('READY','EXPIRING')
    AND dep.state='PANEL_COMPLETE'
);
