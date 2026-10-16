package serverprotection

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"testing"
	"time"
)

type heldReceiptSecrets struct {
	entered chan struct{}
	release chan struct{}
}

func (s heldReceiptSecrets) Get(ctx context.Context, _, _ string) ([]byte, error) {
	close(s.entered)
	select {
	case <-s.release:
		return nil, errors.New("old observation failed")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func TestGuardianShutdownPreservesVerifiedReceipt(t *testing.T) {
	db := pollingDB(t)
	id := pollingSeed(t, db, 701, true)
	pollingExec(t, db, "UPDATE server_protection_nodes SET state='APPLIED',last_error='',checked_at=now()-interval '1 second',verified_at=now()-interval '1 second' WHERE panel_id=$1", id)
	var before time.Time
	if err := db.QueryRow("SELECT checked_at FROM server_protection_nodes WHERE panel_id=$1", id).Scan(&before); err != nil {
		t.Fatal(err)
	}
	secret := heldReceiptSecrets{make(chan struct{}), make(chan struct{})}
	c := Controller{DB: db, Secrets: secret, SSH: provisioning.SSHClient{}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.reconcile(ctx, nodeTarget{Panel: id, Host: "fixture", KeyRef: "fixture", Policy: Policy{PanelID: id, Enabled: true, Revision: 1}})
	}()
	select {
	case <-secret.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("observation not admitted")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("canceled observation did not join")
	}
	var state, last string
	var after time.Time
	if err := db.QueryRow("SELECT state,last_error,checked_at FROM server_protection_nodes WHERE panel_id=$1", id).Scan(&state, &last, &after); err != nil {
		t.Fatal(err)
	}
	if state != "APPLIED" || last != "" || !after.Equal(before) {
		t.Fatal("planned shutdown fabricated observation failure", state, last, before, after)
	}
}
func TestGuardianDeadOwnerCannotOverwriteSuccessor(t *testing.T) {
	db := pollingDB(t)
	id := pollingSeed(t, db, 702, true)
	secret := heldReceiptSecrets{make(chan struct{}), make(chan struct{})}
	c := Controller{DB: db, Secrets: secret, SSH: provisioning.SSHClient{}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.reconcile(ctx, nodeTarget{Panel: id, Host: "fixture", KeyRef: "fixture", Policy: Policy{PanelID: id, Enabled: true, Revision: 1}})
	}()
	select {
	case <-secret.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("observation not admitted")
	}
	var pid int
	key := "server-protection:" + id
	if err := db.QueryRow(`SELECT pid FROM pg_locks WHERE locktype='advisory' AND granted
 AND classid::bigint=((hashtextextended($1,0)>>32)&4294967295)
 AND objid::bigint=(hashtextextended($1,0)&4294967295)`, key).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	var killed bool
	if err := db.QueryRow("SELECT pg_terminate_backend($1)", pid).Scan(&killed); err != nil || !killed {
		t.Fatal(killed, err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", key); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(ctx, "UPDATE server_protection_nodes SET state='APPLIED',last_error='successor marker',checked_at=now(),verified_at=now() WHERE panel_id=$1", id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	close(secret.release)
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("old owner did not join")
	}
	var state, last string
	if err = db.QueryRow("SELECT state,last_error FROM server_protection_nodes WHERE panel_id=$1", id).Scan(&state, &last); err != nil {
		t.Fatal(err)
	}
	if state != "APPLIED" || last != "successor marker" {
		t.Fatal("dead guardian owner overwrote successor", state, last)
	}
}
