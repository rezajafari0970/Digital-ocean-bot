CREATE TABLE bulk_user_shrink_gate (
  singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
  enabled boolean NOT NULL DEFAULT false,
  max_delete_per_inbound integer NOT NULL DEFAULT 32 CHECK(max_delete_per_inbound BETWEEN 1 AND 32),
  updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO bulk_user_shrink_gate(singleton,enabled,max_delete_per_inbound) VALUES(true,false,32);
