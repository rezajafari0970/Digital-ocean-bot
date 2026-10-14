-- Add evidence provenance without rewriting any account's provider state.
ALTER TABLE accounts
 ADD COLUMN provider_observation_started_at timestamptz,
 ADD COLUMN provider_restriction_started_at timestamptz,
 ADD COLUMN provider_success_started_at timestamptz,
 ADD COLUMN provider_error_started_at timestamptz,
 ADD COLUMN provider_error_at timestamptz,
 ADD COLUMN provider_refresh_attempt_at timestamptz,
 ADD COLUMN provider_account_status text,
 ADD COLUMN provider_account_observed_at timestamptz;
