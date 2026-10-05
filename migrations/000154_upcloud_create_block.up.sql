CREATE TABLE IF NOT EXISTS account_create_blocks (
 account_id uuid PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
 version bigserial NOT NULL,
 code text NOT NULL CHECK (code ~ '^[A-Z0-9_]{1,80}$'),
 blocked_at timestamptz NOT NULL DEFAULT now()
);
GRANT SELECT,INSERT,UPDATE,DELETE ON account_create_blocks TO digitaloceanbot;
GRANT USAGE,SELECT ON SEQUENCE account_create_blocks_version_seq TO digitaloceanbot;

-- Only actual, definitive create failures are evidence. A successful read is not.
INSERT INTO account_create_blocks(account_id,code,blocked_at)
SELECT a.id,substring(d.last_error from 'UpCloud HTTP 403 \(([A-Z0-9_]{1,80})\)'),d.updated_at
FROM accounts a
JOIN LATERAL (
 SELECT last_error,updated_at FROM deployments WHERE account_id=a.id
 AND last_error LIKE '%provider permission_denied: UpCloud HTTP 403 (%'
 ORDER BY updated_at DESC LIMIT 1
) d ON true
WHERE a.provider='upcloud' AND a.deleted_at IS NULL
AND substring(d.last_error from 'UpCloud HTTP 403 \(([A-Z0-9_]{1,80})\)') IS NOT NULL
AND NOT EXISTS(SELECT 1 FROM operations o WHERE o.account_id=a.id AND o.kind='CREATE_DROPLET' AND o.state='succeeded' AND o.updated_at>d.updated_at)
AND NOT EXISTS(SELECT 1 FROM audit_events e WHERE e.account_id=a.id AND e.action='create_permission_retry_enabled' AND e.created_at>d.updated_at)
ON CONFLICT(account_id) DO NOTHING;
