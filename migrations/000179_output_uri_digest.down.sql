-- Restoring the old index can fail while long XHTTP links exist. The runner
-- transaction rolls back rather than truncating or deleting any output.
ALTER TABLE output_config_snapshots ADD PRIMARY KEY(panel_id,uri);
DROP INDEX output_config_snapshots_uri_digest_key;
