-- 177 is reserved by the independent IP wire branch. The runner keys each
-- complete filename and applies unapplied files, including later-merged 177.
CREATE TABLE reality_transport_policy (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 config jsonb NOT NULL DEFAULT '{"network":"tcp","finalmask_enabled":false}'::jsonb,
 updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO reality_transport_policy(singleton) VALUES(true);
CREATE TABLE reality_listener_profiles (
 panel_id uuid NOT NULL REFERENCES panel_instances(id) ON DELETE CASCADE,
 managed_key text NOT NULL,
 config jsonb NOT NULL,
 path text,
 policy_revision bigint NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(panel_id,managed_key),
 CHECK(path IS NULL OR (octet_length(path) BETWEEN 256 AND 4096))
);

-- A 4096-byte text value exceeds the btree key limit. The bounded digest
-- index rejects equal paths; a digest collision also fails closed and retries.
CREATE UNIQUE INDEX reality_listener_profiles_path_key ON reality_listener_profiles(md5(path)) WHERE path IS NOT NULL;
