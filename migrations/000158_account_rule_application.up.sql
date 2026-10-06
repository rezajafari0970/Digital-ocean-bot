CREATE TABLE account_rule_application (
 account_id uuid PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
 apply_to_existing boolean NOT NULL DEFAULT false,
 revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
 replacement_revision bigint CHECK (replacement_revision > 0 AND replacement_revision <= revision),
 shrink_pending boolean NOT NULL DEFAULT false,
 waiting_droplet_id uuid REFERENCES droplets(id) ON DELETE SET NULL,
 waiting_reason text NOT NULL DEFAULT '',
 waiting_previous_state text NOT NULL DEFAULT 'READY',
 retirement_started boolean NOT NULL DEFAULT false,
 next_action_at timestamptz,
 status text NOT NULL DEFAULT 'FUTURE_ONLY',
 detail text NOT NULL DEFAULT '',
 updated_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE deployments ADD COLUMN build_rules_revision bigint NOT NULL DEFAULT 0 CHECK (build_rules_revision >= 0);
CREATE INDEX account_rule_application_due ON account_rule_application (next_action_at,updated_at) WHERE apply_to_existing;
