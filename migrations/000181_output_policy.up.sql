CREATE TABLE output_export_policy (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 include_xhttp_extra boolean NOT NULL DEFAULT false,
 include_finalmask boolean NOT NULL DEFAULT false,
 updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO output_export_policy(singleton) VALUES(true);
