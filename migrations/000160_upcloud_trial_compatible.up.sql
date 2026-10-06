ALTER TABLE accounts ADD COLUMN upcloud_trial_compatible boolean NOT NULL DEFAULT false;
ALTER TABLE accounts ADD CONSTRAINT upcloud_trial_provider CHECK(NOT upcloud_trial_compatible OR provider='upcloud');
