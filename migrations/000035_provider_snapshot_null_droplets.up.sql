UPDATE provider_snapshots
SET data = jsonb_set(data, '{Droplets}', '[]'::jsonb, true)
WHERE data->'Droplets' IS NULL OR data->'Droplets' = 'null'::jsonb;

CREATE OR REPLACE FUNCTION normalize_provider_snapshot_droplets()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.data->'Droplets' IS NULL OR NEW.data->'Droplets' = 'null'::jsonb THEN
    NEW.data = jsonb_set(NEW.data, '{Droplets}', '[]'::jsonb, true);
  END IF;
  RETURN NEW;
END
$$;

DROP TRIGGER IF EXISTS trg_normalize_provider_snapshot_droplets ON provider_snapshots;
CREATE TRIGGER trg_normalize_provider_snapshot_droplets
BEFORE INSERT OR UPDATE OF data ON provider_snapshots
FOR EACH ROW EXECUTE FUNCTION normalize_provider_snapshot_droplets();