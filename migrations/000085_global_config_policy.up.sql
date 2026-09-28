CREATE TABLE global_config_policies (
 policy_key text PRIMARY KEY,
 enabled boolean NOT NULL DEFAULT true,
 ports jsonb NOT NULL DEFAULT '[]'::jsonb,
 target_users_per_inbound integer NOT NULL DEFAULT 0 CHECK(target_users_per_inbound >= 0),
 user_quota_bytes bigint NOT NULL DEFAULT 0 CHECK(user_quota_bytes >= 0),
 user_lifetime_seconds integer NOT NULL DEFAULT 0 CHECK(user_lifetime_seconds >= 0),
 device_limit integer NOT NULL DEFAULT 0 CHECK(device_limit >= 0),
 users_per_second integer NOT NULL DEFAULT 1 CHECK(users_per_second >= 1),
 sni_selection_mode text NOT NULL DEFAULT 'scored' CHECK(sni_selection_mode IN ('scored','manual')),
 manual_snis jsonb NOT NULL DEFAULT '[]'::jsonb,
 revision bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
