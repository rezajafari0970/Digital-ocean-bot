CREATE TABLE installer_reboot_remediations (
 deployment_id UUID NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
 generation INTEGER NOT NULL CHECK(generation>0),
 state TEXT NOT NULL CHECK(state IN ('SCHEDULED','FAILED')),
 scheduled_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 last_error TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(deployment_id,generation)
);
