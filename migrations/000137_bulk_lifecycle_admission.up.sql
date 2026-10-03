ALTER TABLE bulk_lifecycle_control ADD COLUMN auto_enroll boolean NOT NULL DEFAULT false;
ALTER TABLE bulk_lifecycle_control ADD COLUMN max_active_scopes integer NOT NULL DEFAULT 64 CHECK(max_active_scopes BETWEEN 1 AND 256);
ALTER TABLE bulk_lifecycle_control ADD COLUMN operation_budget integer NOT NULL DEFAULT 10000 CHECK(operation_budget BETWEEN 1 AND 10000);
ALTER TABLE bulk_lifecycle_control ADD COLUMN max_batch_size integer NOT NULL DEFAULT 100 CHECK(max_batch_size BETWEEN 1 AND 100);
