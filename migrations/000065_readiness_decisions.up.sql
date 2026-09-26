CREATE TABLE IF NOT EXISTS server_readiness_issues (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 readiness_id uuid NOT NULL REFERENCES server_readiness_snapshots(id) ON DELETE CASCADE,
 run_id uuid NOT NULL REFERENCES provision_runs(id) ON DELETE CASCADE,
 code text NOT NULL,
 severity text NOT NULL CHECK(severity IN ('WARN','BLOCK')),
 action text NOT NULL CHECK(action IN ('RETRY','AUTO_REMEDIATE','BLOCK','WARN')),
 remediation text NOT NULL DEFAULT '',
 state text NOT NULL DEFAULT 'OPEN' CHECK(state IN ('OPEN','REMEDIATED','FAILED','OBSERVED')),
 detail text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS server_readiness_issues_run_idx ON server_readiness_issues(run_id,created_at DESC);
CREATE INDEX IF NOT EXISTS server_readiness_issues_code_idx ON server_readiness_issues(code,state,created_at DESC);
