CREATE TABLE IF NOT EXISTS account_browser_identities (
    account_id UUID PRIMARY KEY
        REFERENCES accounts(id) ON DELETE CASCADE,

    profile_namespace TEXT NOT NULL,

    platform TEXT,
    user_agent TEXT,
    timezone TEXT,
    language TEXT,
    screen TEXT,

    hardware_concurrency INTEGER,
    device_memory DOUBLE PRECISION,
    touch_support BOOLEAN,

    canvas_hash TEXT,

    webgl_vendor TEXT,
    webgl_renderer TEXT,
    webgl_hash TEXT,

    audio_hash TEXT,
    client_rects_hash TEXT,
    fonts_hash TEXT,

    webrtc_candidates JSONB NOT NULL DEFAULT '[]'::jsonb,
    webrtc_leak BOOLEAN NOT NULL DEFAULT false,

    raw JSONB NOT NULL DEFAULT '{}'::jsonb,
    checked_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

GRANT SELECT,INSERT,UPDATE,DELETE
ON account_browser_identities
TO digitaloceanbot;
