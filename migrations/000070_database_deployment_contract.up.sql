ALTER TABLE xui_database_deployments
 ADD CONSTRAINT xui_database_deployments_state_check
 CHECK (state IN ('IMPORTING','FAILED','COMPLETED'));

CREATE UNIQUE INDEX xui_database_deployments_droplet_uidx
 ON xui_database_deployments(droplet_id);
