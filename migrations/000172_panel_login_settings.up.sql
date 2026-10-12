-- Retire first-use restrictions without resetting any username or password.
UPDATE admin_users SET must_change_password=false,bootstrap_token_hash=NULL
WHERE must_change_password OR bootstrap_token_hash IS NOT NULL;
UPDATE panel_settings SET listen_host='0.0.0.0' WHERE listen_host<>'0.0.0.0';
ALTER TABLE panel_settings ADD CONSTRAINT panel_listen_host_fixed CHECK(listen_host='0.0.0.0');
