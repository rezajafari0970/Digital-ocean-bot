CREATE TABLE reality_rollout_control (
    policy_key TEXT PRIMARY KEY,
    mode TEXT NOT NULL CHECK (mode IN ('canary','stable')),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO reality_rollout_control(policy_key,mode) VALUES('reality','stable')
ON CONFLICT(policy_key) DO NOTHING;
