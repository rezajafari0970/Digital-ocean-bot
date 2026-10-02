CREATE OR REPLACE FUNCTION bump_account_transport_epoch_for_identity()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  p TEXT;
  px UUID;
BEGIN
  IF (OLD.sticky_session IS DISTINCT FROM NEW.sticky_session)
     OR (OLD.exit_ip IS DISTINCT FROM NEW.exit_ip) THEN
    SELECT a.provider,np.proxy_id
      INTO p,px
      FROM accounts a
      LEFT JOIN network_profiles np ON np.account_id=a.id
     WHERE a.id=NEW.account_id;

    INSERT INTO account_transport_state(account_id,provider,transport_epoch,active_proxy_id,transition_reason)
    VALUES(NEW.account_id,p,1,px,'network-identity-change')
    ON CONFLICT(account_id) DO UPDATE
      SET provider=EXCLUDED.provider,
          transport_epoch=account_transport_state.transport_epoch+1,
          active_proxy_id=EXCLUDED.active_proxy_id,
          transition_reason=EXCLUDED.transition_reason,
          updated_at=now();
  END IF;
  RETURN NEW;
END
$$;
DROP TRIGGER IF EXISTS trg_bump_transport_epoch_identity ON account_network_identities;
CREATE TRIGGER trg_bump_transport_epoch_identity
AFTER UPDATE OF sticky_session,exit_ip
ON account_network_identities
FOR EACH ROW
EXECUTE FUNCTION bump_account_transport_epoch_for_identity();
