-- Client-side export overlay only: no server config, fleet default or inheritance.
CREATE TABLE output_client_transport_profiles (
 panel_id uuid PRIMARY KEY REFERENCES panel_instances(id) ON DELETE CASCADE,
 preset text NOT NULL CHECK (preset IN ('off','tls-record-v1','tcp-balanced-v1','layered-balanced-v1')),
 revision bigint NOT NULL CHECK (revision>0),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE output_client_transport_operations (
 panel_id uuid NOT NULL REFERENCES panel_instances(id) ON DELETE CASCADE,
 operation_id uuid NOT NULL,
 expected_revision bigint NOT NULL,
 preset text NOT NULL,
 result_revision bigint NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(panel_id,operation_id)
);
GRANT SELECT,INSERT,UPDATE,DELETE ON output_client_transport_profiles,output_client_transport_operations TO digitaloceanbot;
