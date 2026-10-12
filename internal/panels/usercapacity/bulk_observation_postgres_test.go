package usercapacity

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestBulkObservationPreservesManualAndClassDeficitPostgres(t *testing.T) {
	dsn := os.Getenv("BULK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.Contains(u.Path, "bulk_test") {
		t.Fatal("isolated test database required")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	id, _ := sanaei.UUIDv4()
	schema := "capacity_" + strings.ReplaceAll(id, "-", "")
	if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec("DROP SCHEMA " + schema + " CASCADE")
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	must := func(s string, args ...any) {
		t.Helper()
		if _, err := db.Exec(s, args...); err != nil {
			t.Fatal(err)
		}
	}
	must(`CREATE TABLE panel_instances(id uuid PRIMARY KEY);
 CREATE TABLE bulk_lifecycle_scopes(panel_id uuid,inbound_id bigint,generation_id uuid);
 CREATE TABLE bulk_user_ownership(generation_id uuid,client_id text,route_class text,state text,mutation_job_id uuid);
 CREATE TABLE bulk_user_generations(id uuid,panel_id uuid,inbound_id bigint);
 CREATE TABLE panel_client_routes(panel_id uuid,client_id text,route_class text);`)
	for _, file := range []string{"000088_user_capacity_snapshots.up.sql", "000148_independent_reality_profiles.up.sql", "000149_profile_creation_interval.up.sql"} {
		raw, err := os.ReadFile("../../../migrations/" + file)
		if err != nil {
			t.Fatal(err)
		}
		ddl := string(raw)
		if strings.Contains(file, "000148") {
			ddl = strings.Split(ddl, "-- Preserve")[0]
		}
		if strings.Contains(file, "000149") {
			ddl = strings.Split(ddl, "-- Seed")[0]
		}
		must(ddl)
	}
	must(`INSERT INTO panel_instances VALUES($1)`, id)
	must(`INSERT INTO bulk_lifecycle_scopes VALUES($1,1,$1);`, id)
	must(`INSERT INTO bulk_user_generations VALUES($1,$1,1)`, id)
	must(`INSERT INTO bulk_user_ownership VALUES($1,'d1','DIRECT','ACTIVE',$1),($1,'d2','DIRECT','ACTIVE',$1),($1,'d3','DIRECT','ACTIVE',$1)`, id)
	must(`INSERT INTO reality_config_profiles(route_class,ports,target_users_per_inbound) VALUES('DIRECT','[443]',1),('RESIDENTIAL','[443]',3)`)
	in := rawInbound{ID: 1, Port: 443, Enable: true, Protocol: "vless", StreamSettings: json.RawMessage(`{"network":"tcp","security":"reality"}`), Settings: json.RawMessage(`{"clients":[{"id":"d1","email":"d1","enable":true},{"id":"d2","email":"d2","enable":true},{"id":"d3","email":"d3","enable":true},{"id":"manual","email":"manual","enable":true}]}`)}
	raw, _ := json.Marshal(in)
	if err = (Service{DB: db}).ObserveVerifiedSnapshot(context.Background(), id, []json.RawMessage{raw}); err != nil {
		t.Fatal(err)
	}
	var target, active, deficit int
	var observed, retained time.Time
	if err = db.QueryRow("SELECT target_users,active_users,deficit,observed_at FROM user_capacity_snapshots WHERE panel_id=$1", id).Scan(&target, &active, &deficit, &observed); err != nil || target != 5 || active != 4 || deficit != 3 {
		t.Fatal(target, active, deficit, err)
	}
	// Native 3x-ui also returns streamSettings as an encoded JSON string.
	// Clear the prior evidence so silently skipping that format cannot pass.
	must("DELETE FROM user_capacity_snapshots WHERE panel_id=$1", id)
	in.StreamSettings, err = json.Marshal(string(in.StreamSettings))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(in)
	if err = (Service{DB: db}).ObserveVerifiedSnapshot(context.Background(), id, []json.RawMessage{raw}); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT target_users,active_users,deficit,observed_at FROM user_capacity_snapshots WHERE panel_id=$1", id).Scan(&target, &active, &deficit, &observed); err != nil || target != 5 || active != 4 || deficit != 3 {
		t.Fatal("encoded stream settings were not observed", target, active, deficit, err)
	}
	// A missing native traffic counter must retain the last successful observation.
	in.Settings = json.RawMessage(`{"clients":[{"id":"manual","email":"manual","enable":true,"totalGB":100}]}`)
	raw, _ = json.Marshal(in)
	if err = (Service{DB: db}).ObserveVerifiedSnapshot(context.Background(), id, []json.RawMessage{raw}); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT active_users,observed_at FROM user_capacity_snapshots WHERE panel_id=$1", id).Scan(&active, &retained); err != nil || active != 4 || !retained.Equal(observed) {
		t.Fatal("failed observation overwrote evidence", active, err)
	}
}
