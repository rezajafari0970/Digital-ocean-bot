CREATE TABLE panel_settings (
 id BOOLEAN PRIMARY KEY DEFAULT true CHECK(id=true),
 listen_host TEXT NOT NULL DEFAULT '0.0.0.0',
 listen_port INTEGER NOT NULL DEFAULT 18080 CHECK(listen_port BETWEEN 1024 AND 65535),
 web_path TEXT NOT NULL DEFAULT '/admin',
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO panel_settings(id) VALUES(true) ON CONFLICT(id) DO NOTHING;
