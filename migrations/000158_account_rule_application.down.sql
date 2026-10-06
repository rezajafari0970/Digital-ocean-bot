-- Disable application and settle active retirements before downgrading binaries.
DROP TABLE account_rule_application;
ALTER TABLE deployments DROP COLUMN build_rules_revision;
