package adminapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/export"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
)

func TestOutputSnapshotLongXHTTPRoundTrip(t *testing.T) {
	db := adminTestDB(t)
	_, _, panel := seedPanel(t, db, "http://panel.test")
	s := &Server{DB: db}
	ctx := context.Background()
	profile := transportprofile.Preset()
	// Production jsonb reads add spaces which must survive strict URI decoding.
	if err := db.QueryRow("SELECT $1::jsonb", string(profile.Extra)).Scan(&profile.Extra); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(profile.Extra), ": ") {
		t.Fatal("fixture did not exercise PostgreSQL JSON whitespace")
	}
	var err error
	profile.Path, err = transportprofile.NewPath(4096)
	if err != nil {
		t.Fatal(err)
	}
	records := []outputRecord{{URI: "vless://legacy@panel.test:443?type=tcp", ClientID: "legacy"}}
	for _, id := range []string{"b530c072-21c6-405d-95d8-fa78afa2d62e", "a530c072-21c6-405d-95d8-fa78afa2d62e"} {
		uri, err := export.VLESSRealityURI(export.VLESSReality{
			Transport: profile, UUID: id, Host: "192.0.2.1", Port: 443,
			SNI: "www.example.com", PublicKey: "public", ShortID: "04a54a16", Remark: "long-path",
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(uri) <= 4096 {
			t.Fatal("fixture must exercise a full-length exported path")
		}
		records = append(records, outputRecord{URI: uri, ClientID: id})
	}
	s.persistOutputSnapshot(ctx, panel, records)
	assertRecords := func(want []outputRecord) {
		t.Helper()
		var count int
		if err := db.QueryRow("SELECT count(*) FROM output_config_snapshots WHERE panel_id=$1", panel).Scan(&count); err != nil || count != len(want) {
			t.Fatalf("persisted rows=%d want=%d err=%v", count, len(want), err)
		}
		for _, rec := range want {
			var uri string
			var bounded bool
			if err := db.QueryRow("SELECT uri,visible_until<=now()+interval '1 hour' FROM output_config_snapshots WHERE panel_id=$1 AND client_id=$2", panel, rec.ClientID).Scan(&uri, &bounded); err != nil || uri != rec.URI || !bounded {
				t.Fatalf("full URI or expiry not preserved: err=%v bounded=%v", err, bounded)
			}
		}
	}
	assertRecords(records)
	var first time.Time
	if err := db.QueryRow("SELECT first_seen_at FROM output_config_snapshots WHERE panel_id=$1 AND client_id='legacy'", panel).Scan(&first); err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, "UPDATE output_config_snapshots SET last_seen_at=now()-interval '1 minute' WHERE panel_id=$1", panel)
	s.persistOutputSnapshot(ctx, panel, append(records, records[1]))
	assertRecords(records)
	var unchanged, fresh bool
	if err := db.QueryRow("SELECT first_seen_at=$2,last_seen_at>now()-interval '15 seconds' FROM output_config_snapshots WHERE panel_id=$1 AND client_id='legacy'", panel, first).Scan(&unchanged, &fresh); err != nil || !unchanged || !fresh {
		t.Fatalf("refresh identity/freshness: %v %v %v", unchanged, fresh, err)
	}
	sqlMust(t, db, "INSERT INTO output_share_tokens(token,route_class) VALUES('long-xhttp-test','ALL')")
	req := httptest.NewRequest("GET", "/share/output/long-xhttp-test", nil)
	req.SetPathValue("token", "long-xhttp-test")
	w := httptest.NewRecorder()
	s.sharedOutput(w, req)
	if w.Code != 200 {
		t.Fatalf("share status %d", w.Code)
	}
	for _, rec := range records {
		if !strings.Contains(w.Body.String(), rec.URI+"\n") {
			t.Fatal("shared output lost a complete URI")
		}
		u, err := url.Parse(rec.URI)
		if err != nil {
			t.Fatal(err)
		}
		foundExtra := false
		for _, field := range strings.Split(u.RawQuery, "&") {
			key, raw, _ := strings.Cut(field, "=")
			if key != "extra" {
				continue
			}
			foundExtra = true
			decoded, err := url.PathUnescape(raw)
			var extra struct {
				NoGRPCHeader bool `json:"noGRPCHeader"`
			}
			if err != nil || json.Unmarshal([]byte(decoded), &extra) != nil || !extra.NoGRPCHeader {
				t.Fatal("shared Extra failed v2rayNG decoding or lost boolean type")
			}
		}
		if u.Query().Get("type") == "xhttp" && !foundExtra {
			t.Fatal("shared XHTTP URI lost the extra field")
		}
	}
	s.persistOutputSnapshot(ctx, panel, records[1:])
	assertRecords(records[1:])
	s.persistOutputSnapshot(ctx, panel, nil)
	assertRecords(nil)
}

func TestOutputSnapshotDigestMigrationPreservesTCP(t *testing.T) {
	db := adminTestDBThrough(t, 178)
	_, _, panel := seedPanel(t, db, "http://panel.test")
	const uri = "vless://legacy@panel.test:443?type=tcp"
	sqlMust(t, db, "INSERT INTO output_config_snapshots(panel_id,uri,client_id,first_seen_at) VALUES($1,$2,'legacy',now()-interval '1 day')", panel, uri)
	var first time.Time
	if err := db.QueryRow("SELECT first_seen_at FROM output_config_snapshots WHERE panel_id=$1", panel).Scan(&first); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../migrations/000179_output_uri_digest.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, string(raw))
	s := &Server{DB: db}
	if err := s.persistOutputSnapshot(context.Background(), panel, []outputRecord{{URI: uri, ClientID: "legacy"}}); err != nil {
		t.Fatal(err)
	}
	var unchanged bool
	if err := db.QueryRow("SELECT uri=$2 AND first_seen_at=$3 FROM output_config_snapshots WHERE panel_id=$1", panel, uri, first).Scan(&unchanged); err != nil || !unchanged {
		t.Fatal("migration changed existing output", err)
	}
	// Force a real database write failure: it must be returned and must not
	// replace the previous complete snapshot with a partial result.
	sqlMust(t, db, `CREATE FUNCTION reject_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN RAISE EXCEPTION 'synthetic write failure'; END $$`)
	sqlMust(t, db, `CREATE TRIGGER reject_snapshot BEFORE INSERT ON output_config_snapshots FOR EACH ROW EXECUTE FUNCTION reject_snapshot()`)
	if err := s.persistOutputSnapshot(context.Background(), panel, []outputRecord{{URI: "vless://new@panel.test:443", ClientID: "new"}}); err == nil {
		t.Fatal("database failure was swallowed")
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM output_config_snapshots WHERE panel_id=$1 AND uri=$2", panel, uri).Scan(&count); err != nil || count != 1 {
		t.Fatal("failed refresh lost prior output", err)
	}
}
