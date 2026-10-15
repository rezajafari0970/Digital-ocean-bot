-- Full XHTTP links exceed the btree key limit. Preserve the complete URI as
-- data, indexing only a bounded digest. The writer verifies full URI equality
-- on conflict and aborts the whole snapshot if a digest collision is observed.
CREATE UNIQUE INDEX output_config_snapshots_uri_digest_key
ON output_config_snapshots(panel_id,md5(uri));
ALTER TABLE output_config_snapshots DROP CONSTRAINT output_config_snapshots_pkey;
