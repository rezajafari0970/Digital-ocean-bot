DROP TRIGGER IF EXISTS trg_bump_transport_epoch_proxy ON proxies;
DROP FUNCTION IF EXISTS bump_account_transport_epoch_for_proxy();
DROP TRIGGER IF EXISTS trg_bump_transport_epoch_profile ON network_profiles;
DROP FUNCTION IF EXISTS bump_account_transport_epoch_for_profile();
