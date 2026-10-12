package adminapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/migrate"
	"golang.org/x/crypto/bcrypt"
)

type fakeUpdateController struct {
	mu      sync.Mutex
	actions []string
	state   updateState
}

func (f *fakeUpdateController) Request(_ context.Context, action string) (updateState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actions = append(f.actions, action)
	return f.state, nil
}
func TestPanelSettingsAtomicCredentials(t *testing.T) {
	db := adminTestDB(t)
	id := perfUUID()
	raw, err := bcrypt.GenerateFromPassword([]byte("settings-fixture-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, "INSERT INTO admin_users(id,username,password_hash,role) VALUES($1,'admin',$2,'admin')", id, string(raw))
	sqlMust(t, db, "INSERT INTO admin_users(id,username,password_hash,role) VALUES($1,'occupied',$2,'admin')", perfUUID(), string(raw))
	svc := auth.Service{Store: auth.SQLStore{DB: db}}
	token, _, err := svc.Login(context.Background(), "admin", "settings-fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	control := &fakeUpdateController{state: updateState{Available: true, Status: "idle"}}
	server := Server{DB: db, Auth: svc, Updater: control}
	routes := server.Routes()
	request := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/api/v1/panel-settings", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		routes.ServeHTTP(w, r)
		return w
	}
	var port int
	var path string
	if err = db.QueryRow("SELECT listen_port,web_path FROM panel_settings").Scan(&port, &path); err != nil {
		t.Fatal(err)
	}
	payload := func(username, password, current string) string {
		raw, _ := json.Marshal(map[string]any{"listen_port": port, "web_path": path, "username": username, "password": password, "current_password": current})
		return string(raw)
	}
	for _, bad := range []string{
		`{"listen_host":"127.0.0.1","listen_port":18080,"web_path":"/admin","username":"admin"}`,
		payload("changed", "", "incorrect"),
		payload("admin", "too-short", "settings-fixture-password"),
	} {
		if w := request(bad); w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w := request(payload("occupied", "", "settings-fixture-password")); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	var count int
	if err = db.QueryRow("SELECT count(*) FROM admin_sessions WHERE user_id=$1", id).Scan(&count); err != nil || count != 1 {
		t.Fatal("invalid save changed sessions", count, err)
	}
	if w := request(payload("renamed", "", "settings-fixture-password")); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var after string
	if err = db.QueryRow("SELECT password_hash FROM admin_users WHERE id=$1", id).Scan(&after); err != nil || after != string(raw) {
		t.Fatal("blank password did not preserve hash")
	}
	if _, err = svc.Authenticate(context.Background(), token); err == nil {
		t.Fatal("rename retained prior session")
	}
	h := sha256.Sum256([]byte("late-old-username-login"))
	if err = (auth.SQLStore{DB: db}).CreateSession(context.Background(), auth.Session{UserID: id, CredentialHash: string(raw), CredentialUsername: "admin", TokenHash: h[:], ExpiresAt: time.Now().Add(time.Hour)}); err == nil {
		t.Fatal("stale old username login accepted")
	}
	token, _, err = svc.Login(context.Background(), "renamed", "settings-fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	if w := request(payload("renamed", "replacement-fixture-password", "settings-fixture-password")); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, _, err = svc.Login(context.Background(), "renamed", "settings-fixture-password"); err == nil {
		t.Fatal("old password accepted")
	}
	if _, _, err = svc.Login(context.Background(), "renamed", "replacement-fixture-password"); err != nil {
		t.Fatal(err)
	}
}

func TestPanelSettingsAddressAndAdminAuthorization(t *testing.T) {
	db := adminTestDB(t)
	raw, _ := bcrypt.GenerateFromPassword([]byte("address-fixture-password"), bcrypt.MinCost)
	for _, role := range []string{"admin", "operator", "viewer"} {
		sqlMust(t, db, "INSERT INTO admin_users(id,username,password_hash,role) VALUES($1,$2,$3,$2)", perfUUID(), role, string(raw))
	}
	svc := auth.Service{Store: auth.SQLStore{DB: db}}
	control := &fakeUpdateController{state: updateState{Available: true, Status: "idle"}}
	s := Server{DB: db, Auth: svc, Updater: control}
	routes := s.Routes()
	call := func(role, method, path, body string) *httptest.ResponseRecorder {
		token, _, err := svc.Login(context.Background(), role, "address-fixture-password")
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		routes.ServeHTTP(w, r)
		return w
	}
	for _, role := range []string{"viewer", "operator"} {
		for _, path := range []string{"/api/v1/panel-settings", "/api/v1/project-update"} {
			if w := call(role, "GET", path, ""); w.Code != 403 {
				t.Fatal(role, path, w.Code)
			}
		}
		if w := call(role, "POST", "/api/v1/project-update", "{}"); w.Code != 403 {
			t.Fatal(role, w.Code)
		}
	}
	if len(control.actions) != 0 {
		t.Fatal("non-admin reached root bridge")
	}
	w := call("admin", "GET", "/api/v1/panel-settings", "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "listen_host") || strings.Contains(w.Body.String(), "password") {
		t.Fatal(w.Code, w.Body.String())
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	body, _ := json.Marshal(map[string]any{"listen_port": port, "web_path": "/custom-panel", "username": "admin", "password": ""})
	w = call("admin", "PUT", "/api/v1/panel-settings", string(body))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"restart_queued":true`) {
		t.Fatal(w.Code, w.Body.String())
	}
	var host, path string
	var saved int
	if err = db.QueryRow("SELECT listen_host,listen_port,web_path FROM panel_settings").Scan(&host, &saved, &path); err != nil || host != "0.0.0.0" || saved != port || path != "/custom-panel" {
		t.Fatal(host, saved, path, err)
	}
	if w = call("admin", "POST", "/api/v1/project-update", `{"command":"anything"}`); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w = call("admin", "POST", "/api/v1/project-update", "{}"); w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestMigrationRetentionActualSchema(t *testing.T) {
	db := adminTestDB(t)
	sqlMust(t, db, "CREATE TABLE retention_fixture(id int, value text); INSERT INTO retention_fixture VALUES(1,'keep'),(1,'keep')")
	for _, query := range []string{
		"DELETE FROM retention_fixture",
		"TRUNCATE retention_fixture",
		"DROP TABLE retention_fixture",
		"ALTER TABLE retention_fixture DROP COLUMN value",
		"UPDATE retention_fixture SET value='overwritten'",
		"DELETE FROM retention_fixture WHERE ctid IN (SELECT ctid FROM retention_fixture LIMIT 1)",
		"COMMIT; DELETE FROM retention_fixture",
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "999999_retention.up.sql"), []byte(query), 0600); err != nil {
			t.Fatal(err)
		}
		if err := (migrate.Runner{DB: db, Dir: dir, PreserveData: true}).Up(context.Background()); err == nil {
			t.Fatal("destructive migration accepted", query)
		}
		var count int
		if err := db.QueryRow("SELECT count(*) FROM retention_fixture WHERE value='keep'").Scan(&count); err != nil || count != 2 {
			t.Fatal("rows lost", query, count, err)
		}
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "999999_retention.up.sql"), []byte("ALTER TABLE retention_fixture ADD COLUMN extra text; INSERT INTO retention_fixture VALUES(2,'new',NULL)"), 0600)
	if err := (migrate.Runner{DB: db, Dir: dir, PreserveData: true}).Up(context.Background()); err != nil {
		t.Fatal("additive migration failed", err)
	}
}

func TestMigrationRetentionSequenceReset(t *testing.T) {
	db := adminTestDB(t)
	sqlMust(t, db, "CREATE SEQUENCE retention_sequence START 100; SELECT nextval('retention_sequence')")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "999999_retention.up.sql"), []byte("SELECT setval('retention_sequence',1)"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := (migrate.Runner{DB: db, Dir: dir, PreserveData: true}).Up(context.Background()); err == nil {
		t.Fatal("nontransactional sequence reset accepted")
	}
	var value int
	if err := db.QueryRow("SELECT last_value FROM retention_sequence").Scan(&value); err != nil || value != 100 {
		t.Fatal("sequence value changed", value, err)
	}
}

func TestMigration172PreservesExistingCredentials(t *testing.T) {
	db := adminTestDBThrough(t, 171)
	raw, _ := bcrypt.GenerateFromPassword([]byte("existing-fixture-password"), bcrypt.MinCost)
	id := perfUUID()
	sqlMust(t, db, "INSERT INTO admin_users(id,username,password_hash,role,must_change_password,bootstrap_token_hash) VALUES($1,'keep-login',$2,'admin',true,$3)", id, string(raw), []byte("obsolete-fixture-grant"))
	sqlMust(t, db, "UPDATE panel_settings SET listen_host='127.0.0.1'")
	dir := t.TempDir()
	data, err := os.ReadFile("../../migrations/000172_panel_login_settings.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "000172_panel_login_settings.up.sql"), data, 0600)
	if err = (migrate.Runner{DB: db, Dir: dir, PreserveData: true}).Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	var name, hash string
	var force bool
	if err = db.QueryRow("SELECT username,password_hash,must_change_password FROM admin_users WHERE id=$1", id).Scan(&name, &hash, &force); err != nil || name != "keep-login" || hash != string(raw) || force {
		t.Fatal("credential preservation", name, force, err)
	}
}
