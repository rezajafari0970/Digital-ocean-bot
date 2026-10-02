CREATE OR REPLACE FUNCTION bump_account_transport_epoch_for_profile() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p TEXT;
BEGIN
  IF (OLD.mode IS DISTINCT FROM NEW.mode) OR (OLD.proxy_id IS DISTINCT FROM NEW.proxy_id) THEN
    SELECT provider INTO p FROM accounts WHERE id=NEW.account_id;
    INSERT INTO account_transport_state(account_id,provider,transport_epoch,active_proxy_id,transition_reason)
    VALUES(NEW.account_id,p,1,NEW.proxy_id,'network-profile-change')
    ON CONFLICT(account_id) DO UPDATE SET provider=EXCLUDED.provider,transport_epoch=account_transport_state.transport_epoch+1,active_proxy_id=EXCLUDED.active_proxy_id,transition_reason=EXCLUDED.transition_reason,updated_at=now();
  END IF;
  RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS trg_bump_transport_epoch_profile ON network_profiles;
CREATE TRIGGER trg_bump_transport_epoch_profile AFTER UPDATE OF mode,proxy_id ON network_profiles FOR EACH ROW EXECUTE FUNCTION bump_account_transport_epoch_for_profile();

CREATE OR REPLACE FUNCTION bump_account_transport_epoch_for_proxy() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF (OLD.type IS DISTINCT FROM NEW.type) OR (OLD.host IS DISTINCT FROM NEW.host) OR (OLD.port IS DISTINCT FROM NEW.port) OR (OLD.username IS DISTINCT FROM NEW.username) OR (OLD.adapter IS DISTINCT FROM NEW.adapter) OR (OLD.secret_ref IS DISTINCT FROM NEW.secret_ref) THEN
    INSERT INTO account_transport_state(account_id,provider,transport_epoch,active_proxy_id,transition_reason)
    SELECT np.account_id,a.provider,1,np.proxy_id,'proxy-definition-change' FROM network_profiles np JOIN accounts a ON a.id=np.account_id WHERE np.proxy_id=NEW.id
    ON CONFLICT(account_id) DO UPDATE SET provider=EXCLUDED.provider,transport_epoch=account_transport_state.transport_epoch+1,active_proxy_id=EXCLUDED.active_proxy_id,transition_reason=EXCLUDED.transition_reason,updated_at=now();
  END IF;
  RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS trg_bump_transport_epoch_proxy ON proxies;
CREATE TRIGGER trg_bump_transport_epoch_proxy AFTER UPDATE OF type,host,port,username,adapter,secret_ref ON proxies FOR EACH ROW EXECUTE FUNCTION bump_account_transport_epoch_for_proxy();
