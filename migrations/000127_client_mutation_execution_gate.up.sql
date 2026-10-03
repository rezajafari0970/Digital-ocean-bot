CREATE TABLE client_mutation_execution_gate (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    enabled boolean NOT NULL DEFAULT false,
    kill_switch boolean NOT NULL DEFAULT true,
    panel_id uuid REFERENCES panel_instances(id) ON DELETE SET NULL,
    inbound_id bigint CHECK (inbound_id IS NULL OR inbound_id > 0),
    concurrency integer NOT NULL DEFAULT 1 CHECK (concurrency = 1),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (panel_id IS NOT NULL OR inbound_id IS NULL)
);

INSERT INTO client_mutation_execution_gate(singleton,enabled,kill_switch,concurrency)
VALUES(true,false,true,1);
