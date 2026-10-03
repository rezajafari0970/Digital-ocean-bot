CREATE TABLE panel_cleanup_jobs(
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 state text NOT NULL DEFAULT 'QUEUED' CHECK(state IN('QUEUED','RUNNING','PAUSED','SUCCEEDED')),
 created_at timestamptz NOT NULL DEFAULT now(),
 completed_at timestamptz
);
CREATE UNIQUE INDEX one_active_panel_cleanup ON panel_cleanup_jobs((true)) WHERE state<>'SUCCEEDED';
CREATE TABLE panel_cleanup_targets(
 job_id uuid NOT NULL REFERENCES panel_cleanup_jobs(id) ON DELETE CASCADE,
 panel_id uuid NOT NULL REFERENCES panel_instances(id) ON DELETE CASCADE,
 state text NOT NULL DEFAULT 'PENDING' CHECK(state IN('PENDING','RUNNING','FAILED','SUCCEEDED')),
 planned boolean NOT NULL DEFAULT false,
 attempts integer NOT NULL DEFAULT 0,
 last_error text NOT NULL DEFAULT '',
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(job_id,panel_id)
);
CREATE TABLE panel_cleanup_clients(
 job_id uuid NOT NULL,
 panel_id uuid NOT NULL,
 email text NOT NULL,
 client_id text NOT NULL,
 confirmed_absent boolean NOT NULL DEFAULT false,
 PRIMARY KEY(job_id,panel_id,email),
 FOREIGN KEY(job_id,panel_id) REFERENCES panel_cleanup_targets(job_id,panel_id) ON DELETE CASCADE
);
CREATE TABLE panel_cleanup_inbounds(
 job_id uuid NOT NULL,
 panel_id uuid NOT NULL,
 inbound_id bigint NOT NULL,
 identity text NOT NULL,
 confirmed_absent boolean NOT NULL DEFAULT false,
 PRIMARY KEY(job_id,panel_id,inbound_id),
 FOREIGN KEY(job_id,panel_id) REFERENCES panel_cleanup_targets(job_id,panel_id) ON DELETE CASCADE
);
