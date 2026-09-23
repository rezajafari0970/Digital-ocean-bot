CREATE INDEX IF NOT EXISTS admin_sessions_user_seen_idx ON admin_sessions(user_id,last_seen_at DESC);
DELETE FROM admin_sessions WHERE expires_at < now();
