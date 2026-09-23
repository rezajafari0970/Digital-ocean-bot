ALTER TABLE accounts ADD COLUMN preferred_regions JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE accounts ADD COLUMN preferred_sizes JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE accounts ADD COLUMN preferred_image TEXT;
