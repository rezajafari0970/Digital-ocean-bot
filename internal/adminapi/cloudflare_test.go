package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/cloudflare"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type cfDriverFixture struct {
	mu                     sync.Mutex
	records                map[string]cloudflare.Record
	creates, deletes       int
	lostCreate, lostDelete bool
}

func (d *cfDriverFixture) Zones(context.Context) ([]cloudflare.Zone, error) {
	return []cloudflare.Zone{{ID: "zone-a", Name: "example.com", Status: "active", Type: "full"}}, nil
}
func (d *cfDriverFixture) Records(_ context.Context, z, n string) ([]cloudflare.Record, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if r, ok := d.records[n]; ok {
		return []cloudflare.Record{r}, nil
	}
	return nil, nil
}
func (d *cfDriverFixture) CreateRecord(_ context.Context, z string, r cloudflare.Record) (cloudflare.Record, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.creates++
	r.ID = "record-" + r.Name
	r.CreatedOn = time.Now()
	d.records[r.Name] = r
	if d.lostCreate {
		return cloudflare.Record{}, errors.New("timeout")
	}
	return r, nil
}
func (d *cfDriverFixture) DeleteRecord(_ context.Context, z, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.deletes++
	for n, r := range d.records {
		if r.ID == id {
			delete(d.records, n)
		}
	}
	if d.lostDelete {
		return errors.New("timeout")
	}
	return nil
}
func (d *cfDriverFixture) Setting(context.Context, string, string) (string, error) {
	return "strict", nil
}
func (d *cfDriverFixture) SetSetting(context.Context, string, string, string) error { return nil }
func (d *cfDriverFixture) Certificates(context.Context, string) ([]cloudflare.Certificate, error) {
	return nil, nil
}
func (d *cfDriverFixture) Issue(context.Context, string, []string) (cloudflare.Certificate, error) {
	return cloudflare.Certificate{}, errors.New("not used")
}
func cfSeed(t *testing.T, db *sql.DB) (string, string, string) {
	t.Helper()
	_, drop, panel := seedPanel(t, db, "http://fixture.invalid")
	account, _ := cloudflare.NewID()
	profile, _ := cloudflare.NewID()
	sqlMust(t, db, "UPDATE deployments SET host='192.0.2.5' WHERE droplet_id=$1", drop)
	sqlMust(t, db, "INSERT INTO cloudflare_accounts(id,name) VALUES($1,'fixture')", account)
	sqlMust(t, db, "INSERT INTO cloudflare_zones(account_id,id,name,status,enabled) VALUES($1,'zone-a','example.com','active',true)", account)
	spec := configdriver.Spec{Protocol: "vless", Network: "ws", Security: "tls", Port: 2083, Path: "/cdn", CloudflareAccount: account, CloudflareRecords: 1, CloudflareInterval: 1800, CloudflareMinLifetime: 2700, CloudflareMaxLifetime: 3600, CloudflareMaxActive: 4}
	raw, _ := json.Marshal(spec)
	sqlMust(t, db, "INSERT INTO config_driver_profiles(id,name,spec,enabled,fleet,classes) VALUES($1,'cdn',$2,true,true,ARRAY['DIRECT'])", profile, raw)
	sqlMust(t, db, "INSERT INTO config_driver_listeners(profile_id,panel_id,managed_key,credential_ref,applied_spec) VALUES($1,$2,$3,'fixture',$4)", profile, panel, "dob:profile:"+profile, raw)
	sqlMust(t, db, "UPDATE cloudflare_ip_refresh SET updated_at=now()")
	return account, profile, panel
}
func TestCloudflareLifecycleLostResponsesAndCleanupWithoutZones(t *testing.T) {
	db := adminTestDB(t)
	account, profile, panel := cfSeed(t, db)
	ctx := context.Background()
	d := &cfDriverFixture{records: map[string]cloudflare.Record{}, lostCreate: true, lostDelete: true}
	s := cloudflare.Service{DB: db, Factory: func(context.Context, string) (cloudflare.Driver, error) { return d, nil }}
	run := func() error {
		sqlMust(t, db, "UPDATE cloudflare_accounts SET next_attempt_at=now()")
		sqlMust(t, db, "UPDATE cloudflare_records SET next_attempt_at=now()")
		return s.RunOne(ctx)
	}
	if e := run(); e == nil {
		t.Fatal("lost create was accepted")
	}
	if d.creates != 1 {
		t.Fatal(d.creates)
	}
	var id, host, state string
	var expiry time.Time
	if e := db.QueryRow("SELECT id::text,hostname,state,expires_at FROM cloudflare_records").Scan(&id, &host, &state, &expiry); e != nil {
		t.Fatal(e)
	}
	if state != "SENT" || expiry.Before(time.Now().Add(44*time.Minute)) || expiry.After(time.Now().Add(time.Hour)) {
		t.Fatal(state, expiry)
	}
	if e := run(); e != nil {
		t.Fatal(e)
	}
	if d.creates != 1 {
		t.Fatal("blind replay")
	}
	var after time.Time
	if e := db.QueryRow("SELECT state,expires_at FROM cloudflare_records WHERE id=$1", id).Scan(&state, &after); e != nil {
		t.Fatal(e)
	}
	if state != "DNS_READY" || after.Before(expiry) || after.Sub(expiry) > time.Second {
		t.Fatal("recovery changed lifetime", state)
	}
	endpoint, e := s.ProbeEndpoint(ctx, account, profile, panel)
	if e != nil || endpoint.ID != id || endpoint.Address != host {
		t.Fatal("endpoint", e)
	}
	points, e := s.Endpoints(ctx, account, profile, panel, 1)
	if e != nil || len(points) != 0 {
		t.Fatal("unverified publication", e)
	}
	tx, e := db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	if e = cloudflare.ConfirmEndpoint(ctx, tx, id, 1, []string{"DIRECT"}); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	points, e = s.Endpoints(ctx, account, profile, panel, 1)
	if e != nil || len(points) != 1 {
		t.Fatal("proof missing", e)
	}
	// A domain deselection must not strand cleanup behind admission of new records.
	sqlMust(t, db, "UPDATE cloudflare_zones SET enabled=false")
	sqlMust(t, db, "UPDATE cloudflare_schedules SET next_batch_at=now()")
	_ = run()
	if d.deletes != 1 {
		t.Fatal("cleanup blocked by no zones", d.deletes)
	}
	_ = run()
	if d.deletes != 1 {
		t.Fatal("blind delete replay")
	}
	if e = db.QueryRow("SELECT state FROM cloudflare_records WHERE id=$1", id).Scan(&state); e != nil || state != "DELETED" {
		t.Fatal(state, e)
	}
}
func TestCloudflareForeignRecordIsNeverDeleted(t *testing.T) {
	db := adminTestDB(t)
	_, _, _ = cfSeed(t, db)
	ctx := context.Background()
	d := &cfDriverFixture{records: map[string]cloudflare.Record{}}
	s := cloudflare.Service{DB: db, Factory: func(context.Context, string) (cloudflare.Driver, error) { return d, nil }}
	if e := s.RunOne(ctx); e != nil {
		t.Fatal(e)
	}
	for n, r := range d.records {
		r.Comment = "foreign-owner"
		d.records[n] = r
	}
	sqlMust(t, db, "UPDATE cloudflare_accounts SET next_attempt_at=now()")
	sqlMust(t, db, "UPDATE cloudflare_records SET expires_at=now()-interval '1 second',next_attempt_at=now()")
	if e := s.RunOne(ctx); e == nil || e.Error() != "cloudflare_record_conflict" {
		t.Fatal(e)
	}
	if d.deletes != 0 {
		t.Fatal("deleted foreign record")
	}
}
func TestCloudflareSecretsIsolationAndKeyVersions(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	a, _ := cloudflare.NewID()
	b, _ := cloudflare.NewID()
	sqlMust(t, db, "INSERT INTO cloudflare_accounts(id,name) VALUES($1,'a'),($2,'b')", a, b)
	store, e := secrets.NewStore(secrets.SQLRepository{DB: db}, make([]byte, 32), 1)
	if e != nil {
		t.Fatal(e)
	}
	if e = store.PutCloudflareTx(ctx, db, a, []byte("account-a-token")); e != nil {
		t.Fatal(e)
	}
	if _, e = store.GetCloudflare(ctx, b); e == nil {
		t.Fatal("cross-account secret")
	}
	var raw []byte
	if e = db.QueryRow("SELECT ciphertext FROM cloudflare_secrets WHERE account_id=$1", a).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(raw), "account-a-token") {
		t.Fatal("plaintext token")
	}
	rotated, e := secrets.NewStoreWithKeys(secrets.SQLRepository{DB: db}, []byte(strings.Repeat("b", 32)), 2, map[int][]byte{1: make([]byte, 32)})
	if e != nil {
		t.Fatal(e)
	}
	got, e := rotated.GetCloudflare(ctx, a)
	if e != nil || string(got) != "account-a-token" {
		t.Fatal("retained key", e)
	}
	sqlMust(t, db, "UPDATE cloudflare_secrets SET account_id=$2 WHERE account_id=$1", a, b)
	if _, e = rotated.GetCloudflare(ctx, b); e == nil {
		t.Fatal("AAD binding missing")
	}
}
func TestCloudflareStatusNeverReturnsCredentials(t *testing.T) {
	db := adminTestDB(t)
	account, _, _ := cfSeed(t, db)
	store, e := secrets.NewStore(secrets.SQLRepository{DB: db}, make([]byte, 32), 1)
	if e != nil {
		t.Fatal(e)
	}
	if e = store.PutCloudflareTx(context.Background(), db, account, []byte("secret-fixture")); e != nil {
		t.Fatal(e)
	}
	s := Server{DB: db}
	w := httptest.NewRecorder()
	s.cloudflareStatus(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || strings.Contains(w.Body.String(), "secret-fixture") || strings.Contains(w.Body.String(), "ciphertext") {
		t.Fatal("credential exposure", w.Code)
	}
}

func TestCloudflareOutputRevocationAndRecordOwnership(t *testing.T) {
	db := adminTestDB(t)
	account, profile, panel := cfSeed(t, db)
	ctx := context.Background()
	s := Server{DB: db}
	sqlMust(t, db, "UPDATE residential_routing_control SET enabled=true,fleet=true")
	sqlMust(t, db, "INSERT INTO panel_routing_state(panel_id,revision,state,verified_at) SELECT $1,revision,'APPLIED',now() FROM residential_routing_control", panel)
	sqlMust(t, db, "UPDATE config_driver_listeners SET state='VERIFIED',applied_revision=1,verified_at=now(),certificate_not_after=now()+interval '7 days' WHERE profile_id=$1", profile)
	sqlMust(t, db, "INSERT INTO config_driver_class_proofs(profile_id,panel_id,route_class,verified_at) VALUES($1,$2,'DIRECT',now())", profile, panel)
	sqlMust(t, db, "INSERT INTO config_driver_clients(profile_id,panel_id,client_id,email,route_class,confirmed) VALUES($1,$2,'cf-owned','cf-owned','DIRECT',true)", profile, panel)
	sqlMust(t, db, "INSERT INTO panel_client_routes(panel_id,client_id,email,route_class,effective_class,revision) SELECT $1,'cf-owned','cf-owned','DIRECT','DIRECT',revision FROM residential_routing_control", panel)
	sqlMust(t, db, "UPDATE cloudflare_zones SET ssl_strict=true,checked_at=now()")
	record, _ := cloudflare.NewID()
	sqlMust(t, db, "INSERT INTO cloudflare_records(id,account_id,zone_id,profile_id,panel_id,hostname,origin_ip,state,expires_at,observed_at,verified_at,verified_revision,verified_classes) VALUES($1,$2,'zone-a',$3,$4,'owned.example.com','192.0.2.5','DNS_READY',now()+interval '45 minutes',now(),now(),1,ARRAY['DIRECT'])", record, account, profile, panel)
	uri := "vless://cf-owned@owned.example.com:2083?security=tls&type=ws&sni=owned.example.com&host=owned.example.com"
	if e := s.persistOutputSnapshot(ctx, panel, []outputRecord{{CloudflareRecordID: record, ProfileID: profile, ProfileRevision: 1, ClientID: "cf-owned", URI: uri}}); e != nil {
		t.Fatal(e)
	}
	read := func(want bool) {
		t.Helper()
		w := httptest.NewRecorder()
		s.outputSnapshotClass(w, httptest.NewRequest("GET", "/", nil), "DIRECT")
		if w.Code != 200 || strings.Contains(w.Body.String(), "owned.example.com") != want {
			t.Fatal("publication fence", w.Code, w.Body.String())
		}
		status := httptest.NewRecorder()
		s.cloudflareStatus(status, httptest.NewRequest("GET", "/", nil))
		var body struct {
			Accounts []struct {
				Records []struct {
					ID    string
					Probe bool `json:"probe_ready"`
				}
			}
		}
		if e := json.Unmarshal(status.Body.Bytes(), &body); e != nil || status.Code != 200 {
			t.Fatal("probe readiness query", e, status.Code)
		}
		found := false
		for _, a := range body.Accounts {
			for _, r := range a.Records {
				if r.ID == record {
					found = true
					if r.Probe != want {
						t.Fatal("probe diverged from output publication")
					}
				}
			}
		}
		if !found {
			t.Fatal("tracked record missing")
		}

	}
	read(true)
	for _, x := range []struct{ set, restore string }{
		{"UPDATE cloudflare_records SET expires_at=now()", "UPDATE cloudflare_records SET expires_at=now()+interval '45 minutes'"},
		{"UPDATE cloudflare_records SET verified_revision=2", "UPDATE cloudflare_records SET verified_revision=1"},
		{"UPDATE cloudflare_records SET verified_classes='{}'", "UPDATE cloudflare_records SET verified_classes=ARRAY['DIRECT']"},
		{"UPDATE cloudflare_accounts SET enabled=false", "UPDATE cloudflare_accounts SET enabled=true"},
		{"UPDATE cloudflare_zones SET enabled=false", "UPDATE cloudflare_zones SET enabled=true"},
		{"UPDATE cloudflare_zones SET ssl_strict=false", "UPDATE cloudflare_zones SET ssl_strict=true"},
		{"UPDATE cloudflare_zones SET checked_at=now()-interval '11 minutes'", "UPDATE cloudflare_zones SET checked_at=now()"},
	} {
		sqlMust(t, db, x.set)
		read(false)
		sqlMust(t, db, x.restore)
		read(true)
	}
	foreign, _ := cloudflare.NewID()
	sqlMust(t, db, "INSERT INTO cloudflare_accounts(id,name) VALUES($1,'foreign')", foreign)
	sqlMust(t, db, "UPDATE config_driver_profiles SET spec=jsonb_set(spec,'{cloudflare_account}',to_jsonb($2::text)) WHERE id=$1", profile, foreign)
	read(false)
}
func TestCloudflareOperatorMeasurementRangeFreshnessAndIdempotency(t *testing.T) {
	db := adminTestDB(t)
	account, profile, panel := cfSeed(t, db)
	ctx := context.Background()
	record, _ := cloudflare.NewID()
	id, _ := cloudflare.NewID()
	sqlMust(t, db, "INSERT INTO cloudflare_records(id,account_id,zone_id,profile_id,panel_id,hostname,origin_ip,state,expires_at) VALUES($1,$2,'zone-a',$3,$4,'probe.example.com','192.0.2.5','DNS_READY',now()+interval '45 minutes')", record, account, profile, panel)
	sqlMust(t, db, "INSERT INTO cloudflare_ip_ranges(network) VALUES('104.16.0.0/13')")
	svc := cloudflare.Service{DB: db}
	m := cloudflare.Measurement{ID: id, RecordID: record, Operator: "mci", IP: "104.16.0.1", LatencyMS: 150, MeasuredAt: time.Now(), ProtocolVerified: true}
	if e := svc.SaveMeasurement(ctx, m); e != nil {
		t.Fatal(e)
	}
	if e := svc.SaveMeasurement(ctx, m); e != nil {
		t.Fatal(e)
	}
	var n int
	db.QueryRow("SELECT count(*) FROM cloudflare_ip_measurements").Scan(&n)
	if n != 1 {
		t.Fatal("duplicate measurement", n)
	}
	for _, change := range []func(*cloudflare.Measurement){func(x *cloudflare.Measurement) { x.IP = "8.8.8.8" }, func(x *cloudflare.Measurement) { x.ProtocolVerified = false }, func(x *cloudflare.Measurement) { x.Operator = "invented" }, func(x *cloudflare.Measurement) { x.MeasuredAt = time.Now().Add(-2 * time.Hour) }} {
		bad := m
		change(&bad)
		if svc.SaveMeasurement(ctx, bad) == nil {
			t.Fatal("invalid probe accepted")
		}
	}
}
