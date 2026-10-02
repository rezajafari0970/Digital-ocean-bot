CREATE OR REPLACE FUNCTION enforce_account_runtime_provider_state() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.runtime_status='READY' AND (NEW.provider_state<>'ACTIVE' OR COALESCE(NEW.provider_error_state,'')<>'') THEN
    NEW.runtime_status='PROVIDER_'||COALESCE(NULLIF(NEW.provider_error_state,''),NEW.provider_state,'TRANSPORT_ERROR');
    NEW.runtime_status_detail=COALESCE(NEW.provider_error_detail,NEW.provider_state_detail,NEW.runtime_status_detail);
    NEW.runtime_status_at=now();
  END IF;
  RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS trg_enforce_account_runtime_provider_state ON accounts;
CREATE TRIGGER trg_enforce_account_runtime_provider_state BEFORE INSERT OR UPDATE OF runtime_status,provider_state,provider_error_state ON accounts FOR EACH ROW EXECUTE FUNCTION enforce_account_runtime_provider_state();

UPDATE accounts SET runtime_status='PROVIDER_'||COALESCE(NULLIF(provider_error_state,''),provider_state,'TRANSPORT_ERROR'),runtime_status_detail=COALESCE(provider_error_detail,provider_state_detail,runtime_status_detail),runtime_status_at=now(),updated_at=now()
WHERE runtime_status='READY' AND (provider_state<>'ACTIVE' OR COALESCE(provider_error_state,'')<>'');
