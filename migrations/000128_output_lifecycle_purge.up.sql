CREATE OR REPLACE FUNCTION purge_output_snapshots_on_droplet_retire()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.state IN ('RETIRING','DELETING','DELETED')
     AND OLD.state IS DISTINCT FROM NEW.state THEN
    DELETE FROM output_config_snapshots o
    USING panel_instances p
    WHERE p.droplet_id=NEW.id AND o.panel_id=p.id;
  END IF;
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS droplets_purge_output_snapshots ON droplets;
CREATE TRIGGER droplets_purge_output_snapshots
AFTER UPDATE OF state ON droplets
FOR EACH ROW
WHEN (NEW.state IN ('RETIRING','DELETING','DELETED'))
EXECUTE FUNCTION purge_output_snapshots_on_droplet_retire();
