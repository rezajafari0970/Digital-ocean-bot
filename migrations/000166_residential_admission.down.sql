DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM residential_performance_panels WHERE COALESCE(jsonb_array_length(config->'excluded_proxy_ids'),0)>0)
 OR EXISTS(SELECT 1 FROM residential_performance_experiments WHERE tuning->>'phase' IN('TESTING','RESTORING','PUBLISHING') AND tuning->>'admission_evidence_id' IS NOT NULL) THEN
  RAISE EXCEPTION 'Restore admission assignments before downgrade';
 END IF;
END $$;
ALTER TABLE residential_performance_operations DROP COLUMN admission_evidence_id;
DROP TABLE residential_admission_evidence;
DROP FUNCTION residential_admission_immutable();
DROP TRIGGER residential_admission_secret_version ON residential_proxy_secrets;
DROP FUNCTION residential_admission_secret_version();
DROP TRIGGER residential_admission_endpoint_version ON residential_proxies;
DROP FUNCTION residential_admission_endpoint_version();
ALTER TABLE residential_proxies DROP COLUMN admission_version;
