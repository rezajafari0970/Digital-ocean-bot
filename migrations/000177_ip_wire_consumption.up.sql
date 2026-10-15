-- Authorization consumption is durable before a transport callback can run.
-- It does not prove that bytes reached a provider. Lost handles are never rebuilt.
CREATE TABLE ip_wire_consumptions (
 operation_id uuid PRIMARY KEY REFERENCES ip_mutation_intents(id),
 dispatch_id uuid NOT NULL UNIQUE,
 request_digest text NOT NULL CHECK(length(request_digest)=64),
 wire_digest text NOT NULL CHECK(length(wire_digest)=64),
 generation text NOT NULL CHECK(generation<>''),
 request jsonb NOT NULL,
 consumed_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE ip_wire_results (
 operation_id uuid PRIMARY KEY REFERENCES ip_wire_consumptions(operation_id),
 body jsonb NOT NULL,
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TRIGGER ip_wire_consumption_immutable BEFORE UPDATE OR DELETE ON ip_wire_consumptions FOR EACH ROW EXECUTE FUNCTION ip_reject_ledger_change();
CREATE TRIGGER ip_wire_result_immutable BEFORE UPDATE OR DELETE ON ip_wire_results FOR EACH ROW EXECUTE FUNCTION ip_reject_ledger_change();
