ALTER TABLE accounts DROP COLUMN IF EXISTS auto_max_concurrent;
ALTER TABLE accounts DROP COLUMN IF EXISTS auto_batch_size;
ALTER TABLE accounts DROP COLUMN IF EXISTS auto_interval_seconds;
ALTER TABLE accounts DROP COLUMN IF EXISTS preferred_region;
