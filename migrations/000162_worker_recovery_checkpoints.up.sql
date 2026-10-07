CREATE TABLE worker_recovery_checkpoints (
 kind text NOT NULL CHECK (kind IN ('operation','deployment','lifecycle')),
 item_id text NOT NULL,
 account_id uuid NOT NULL,
 attempt_id uuid NOT NULL DEFAULT gen_random_uuid(),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(kind,item_id)
);
CREATE INDEX worker_recovery_checkpoints_created_idx ON worker_recovery_checkpoints(created_at,kind,item_id);
