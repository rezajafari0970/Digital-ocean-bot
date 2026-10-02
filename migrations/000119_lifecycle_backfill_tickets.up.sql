ALTER TABLE droplets
  ADD COLUMN IF NOT EXISTS backfill_required boolean NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS droplets_backfill_pending_idx
  ON droplets(account_id, updated_at, id)
  WHERE state='DELETED' AND backfill_required=true AND replacement_deployment_id IS NULL;

-- Seed only the current proven deficit for enabled accounts. This repairs
-- underfill that existed before durable backfill tickets were introduced.
WITH deficit AS (
  SELECT a.id AS account_id,
         GREATEST(
           a.desired_server_count
           - (SELECT count(*) FROM droplets d WHERE d.account_id=a.id AND d.state<>'DELETED')
           - (SELECT count(*) FROM deployments p WHERE p.account_id=a.id
              AND p.state NOT IN ('READY','FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK','PANEL_COMPLETE')
              AND p.droplet_id IS NULL),
           0
         ) AS n
  FROM accounts a
  WHERE a.enabled=true
), candidates AS (
  SELECT d.id,d.account_id,
         row_number() OVER (PARTITION BY d.account_id ORDER BY d.updated_at DESC,d.id) AS rn
  FROM droplets d
  JOIN deficit x ON x.account_id=d.account_id AND x.n>0
  WHERE d.state='DELETED' AND d.replacement_deployment_id IS NULL
)
UPDATE droplets d
SET backfill_required=true
FROM candidates c
JOIN deficit x ON x.account_id=c.account_id
WHERE d.id=c.id AND c.rn<=x.n;
