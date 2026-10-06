DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM accounts WHERE upcloud_trial_compatible)
 OR EXISTS(SELECT 1 FROM deployments d LEFT JOIN droplets r ON r.id=d.droplet_id WHERE d.profile_snapshot->>'upcloud_trial_compatible'='true' AND (d.state NOT IN ('READY','FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK','PANEL_COMPLETE') OR (r.id IS NOT NULL AND r.state<>'DELETED'))) THEN
 RAISE EXCEPTION 'Trial-compatible accounts and live deployments require this version; disable/clean them before downgrade';
 END IF;
END $$;
ALTER TABLE accounts DROP CONSTRAINT upcloud_trial_provider;
ALTER TABLE accounts DROP COLUMN upcloud_trial_compatible;
