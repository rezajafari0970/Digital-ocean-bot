CREATE UNIQUE INDEX IF NOT EXISTS secrets_owner_key_id_idx
ON secrets(owner_key,id);
