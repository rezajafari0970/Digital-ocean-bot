package adminapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"golang.org/x/crypto/bcrypt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBootstrapPasswordAccessFlow(t *testing.T) {
	db := adminTestDB(t)
	hash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	grant := "test-only-generated-bootstrap-grant"
	gh := sha256.Sum256([]byte(grant))
	id := perfUUID()
	sqlMust(t, db, "INSERT INTO admin_users(id,username,password_hash,role,enabled,must_change_password,bootstrap_token_hash) VALUES($1,'admin',$2,'admin',true,true,$3)", id, string(hash), gh[:])
	svc := auth.Service{Store: auth.SQLStore{DB: db}}
	if _, _, err = svc.Login(context.Background(), "admin", "admin"); err == nil {
		t.Fatal("unprotected default login")
	}
	s := &Server{DB: db, Auth: svc}
	routes := s.Routes()
	request := func(method, path, body, token, setup string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-Bootstrap-Token", setup)
		routes.ServeHTTP(w, r)
		return w
	}
	w := request("POST", "/api/v1/auth/login", "{\"username\":\"admin\",\"password\":\"admin\"}", "", grant)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result struct {
		Token string
		User  auth.Principal
	}
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || !result.User.MustChangePassword {
		t.Fatal("setup state missing")
	}
	other, _, err := svc.LoginWithBootstrapGrant(context.Background(), "admin", "admin", grant)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/accounts", "/api/v1/output", "/api/v1/configs"} {
		if w = request("GET", path, "", result.Token, ""); w.Code != 403 || !strings.Contains(w.Body.String(), "password_change_required") {
			t.Fatal(path, w.Code)
		}
	}
	if w = request("GET", "/api/v1/auth/me", "", result.Token, ""); w.Code != 200 {
		t.Fatal("me unavailable")
	}
	for _, body := range []string{
		"{\"current_password\":\"admin\",\"new_password\":\"short\"}",
		"{\"current_password\":\"wrong\",\"new_password\":\"a-valid-new-password\"}",
		"{\"current_password\":\"admin\",\"new_password\":\"a-valid-new-password\",\"extra\":1}",
		"{\"current_password\":\"admin\",\"new_password\":\"a-valid-new-password\"}{}",
	} {
		if w = request("POST", "/api/v1/auth/password", body, result.Token, ""); w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	w = request("POST", "/api/v1/auth/password", "{\"current_password\":\"admin\",\"new_password\":\"a-valid-new-password\"}", result.Token, "")
	if w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, token := range []string{result.Token, other} {
		if _, err = svc.Authenticate(context.Background(), token); err == nil {
			t.Fatal("old session survived")
		}
	}
	if _, _, err = svc.LoginWithBootstrapGrant(context.Background(), "admin", "admin", grant); err == nil {
		t.Fatal("old credentials survived")
	}
	token, p, err := svc.Login(context.Background(), "admin", "a-valid-new-password")
	if err != nil || p.MustChangePassword {
		t.Fatal("new login", err, p)
	}
	if w = request("GET", "/api/v1/auth/me", "", token, ""); w.Code != 200 {
		t.Fatal("new session failed")
	}
	var pending bool
	var savedGrant []byte
	if err = db.QueryRow("SELECT must_change_password,bootstrap_token_hash FROM admin_users WHERE id=$1", id).Scan(&pending, &savedGrant); err != nil || pending || len(savedGrant) != 0 {
		t.Fatal("grant not revoked", err)
	}
	// Simulates a login that verified the old password before the reset committed.
	h := sha256.Sum256([]byte("stale-login-token"))
	err = (auth.SQLStore{DB: db}).CreateSession(context.Background(), auth.Session{UserID: id, CredentialHash: string(hash), TokenHash: h[:], ExpiresAt: time.Now().Add(time.Hour)})
	if err == nil {
		t.Fatal("stale verified login created a session after reset")
	}
}

func TestPasswordResetCoordinatesBlockedLogin(t *testing.T) {
	db := adminTestDB(t)
	id := perfUUID()
	hash, err := auth.HashPassword("existing-password")
	if err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, "INSERT INTO admin_users(id,username,password_hash,role) VALUES($1,'existing',$2,'admin')", id, hash)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec("SELECT id FROM admin_users WHERE id=$1 FOR UPDATE", id); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	h := sha256.Sum256([]byte("blocked-login"))
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result <- (auth.SQLStore{DB: db}).CreateSession(ctx, auth.Session{UserID: id, CredentialHash: hash, TokenHash: h[:], ExpiresAt: time.Now().Add(time.Hour)})
	}()
	select {
	case e := <-result:
		t.Fatal("login did not respect user lock", e)
	case <-time.After(80 * time.Millisecond):
	}
	next, err := auth.HashPassword("replacement-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec("UPDATE admin_users SET password_hash=$2 WHERE id=$1", id, next); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec("DELETE FROM admin_sessions WHERE user_id=$1", id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if e := <-result; e == nil {
		t.Fatal("blocked stale login succeeded")
	}
	var count int
	if err = db.QueryRow("SELECT count(*) FROM admin_sessions WHERE user_id=$1", id).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	svc := auth.Service{Store: auth.SQLStore{DB: db}}
	_, p, err := svc.Login(context.Background(), "existing", "replacement-password")
	if err != nil || p.MustChangePassword {
		t.Fatal("existing user unexpectedly requires setup", err)
	}
}
