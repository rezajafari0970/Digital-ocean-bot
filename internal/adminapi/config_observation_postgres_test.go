package adminapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

func waitConfigQueue(t *testing.T, db *sql.DB, key string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for ctx.Err() == nil {
		var waiting bool
		err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND NOT granted
 AND classid::bigint=((hashtextextended($1,0)>>32)&4294967295)
 AND objid::bigint=(hashtextextended($1,0)&4294967295))`, key).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("no real PostgreSQL queued waiter", key)
}
func receiveConfig(t *testing.T, ch <-chan error) {
	t.Helper()
	select {
	case err := <-ch:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("configuration worker did not join")
	}
}
func awaitConfig(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("configuration stage did not start")
	}
}
func configGate(ctx context.Context, ch <-chan struct{}) error {
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestConfigObservationQueuedWriterProgress(t *testing.T) {
	t.Setenv("DOB_CONFIG_OBSERVATION_PANELS", "all")
	db := adminTestDB(t)
	db.SetMaxOpenConns(8)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	panel := "proof-turnstile"
	proofReady, proofRelease := make(chan struct{}), make(chan struct{})
	proofDone := make(chan error, 1)
	go func() {
		proofDone <- sanaei.WithConfigLock(ctx, db, panel, func(c context.Context) error {
			if !sanaei.ConfigLockHeld(c, db, panel) {
				return errors.New("mutation authority missing")
			}
			captured := c
			if err := sanaei.ConfigProofOnly(c, db, panel); err != nil {
				return err
			}
			if sanaei.ConfigLockHeld(captured, db, panel) {
				return errors.New("captured context retained mutation authority")
			}
			if err := sanaei.ConfigProofOnly(c, db, panel); err == nil {
				return errors.New("second downgrade accepted")
			}
			close(proofReady)
			return configGate(c, proofRelease)
		})
	}()
	awaitConfig(t, proofReady)
	readerReady, readerRelease := make(chan struct{}), make(chan struct{})
	readerDone := make(chan error, 1)
	go func() {
		acquired, err := sanaei.TryWithConfigReadLock(ctx, db, panel, func(c context.Context) error {
			if sanaei.ConfigLockHeld(c, db, panel) {
				return errors.New("reader granted mutation authority")
			}
			close(readerReady)
			return configGate(c, readerRelease)
		})
		if err == nil && !acquired {
			err = errors.New("proof blocked observation")
		}
		readerDone <- err
	}()
	awaitConfig(t, readerReady)
	writerEntered, writerRelease := make(chan struct{}), make(chan struct{})
	writerDone := make(chan error, 1)
	go func() {
		writerDone <- sanaei.WithConfigLock(ctx, db, panel, func(c context.Context) error {
			close(writerEntered)
			return configGate(c, writerRelease)
		})
	}()
	waitConfigQueue(t, db, "panel-config:"+panel)
	// This must use a new session after the exclusive writer is already queued.
	for i := 0; i < 3; i++ {
		got, err := sanaei.TryWithConfigReadLock(ctx, db, panel, func(context.Context) error { return nil })
		if err != nil || !got {
			t.Fatal("queued turnstile writer blocked proof observation", got, err)
		}
	}
	close(proofRelease)
	receiveConfig(t, proofDone)
	waitConfigQueue(t, db, "panel-config-observe:"+panel)
	// PostgreSQL's observation-barrier waiter stops a continuous reader stream.
	for i := 0; i < 10; i++ {
		got, err := sanaei.TryWithConfigReadLock(ctx, db, panel, func(context.Context) error { return nil })
		if err != nil || got {
			t.Fatal("new reader bypassed queued barrier writer", got, err)
		}
	}
	close(readerRelease)
	receiveConfig(t, readerDone)
	awaitConfig(t, writerEntered)
	got, err := sanaei.TryWithConfigReadLock(ctx, db, panel, func(context.Context) error { return nil })
	if err != nil || got {
		t.Fatal("structural writer did not exclude output", got, err)
	}
	close(writerRelease)
	receiveConfig(t, writerDone)
}

func TestConfigObservationLeaseLossCannotPublish(t *testing.T) {
	t.Setenv("DOB_CONFIG_OBSERVATION_PANELS", "all")
	db := adminTestDB(t)
	db.SetMaxOpenConns(6)
	sqlMust(t, db, "CREATE TABLE config_lease_test(value integer)")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pid := make(chan int, 1)
	lost := make(chan struct{})
	resume := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- sanaei.WithConfigLock(ctx, db, "lease-loss", func(c context.Context) error {
			var id int
			if err := sanaei.WithConfigEvidenceTx(c, db, "lease-loss", func(tx *sql.Tx) error { return tx.QueryRowContext(c, "SELECT pg_backend_pid()").Scan(&id) }); err != nil {
				return err
			}
			if err := sanaei.ConfigProofOnly(c, db, "lease-loss"); err != nil {
				return err
			}
			pid <- id
			<-c.Done()
			close(lost)
			if err := configGate(ctx, resume); err != nil {
				return err
			}
			if sanaei.ConfigLockHeld(context.WithoutCancel(c), db, "lease-loss") {
				return errors.New("dead lease kept authority")
			}
			err := sanaei.WithConfigEvidenceTx(context.WithoutCancel(c), db, "lease-loss", func(tx *sql.Tx) error {
				_, err := tx.Exec("INSERT INTO config_lease_test VALUES(1)")
				return err
			})
			if err == nil {
				return errors.New("dead lease published")
			}
			return nil
		})
	}()
	id := <-pid
	var terminated bool
	if err := db.QueryRowContext(ctx, "SELECT pg_terminate_backend($1)", id).Scan(&terminated); err != nil || !terminated {
		t.Fatal(terminated, err)
	}
	awaitConfig(t, lost)
	if err := sanaei.WithConfigLock(ctx, db, "lease-loss", func(c context.Context) error {
		return sanaei.WithConfigEvidenceTx(c, db, "lease-loss", func(tx *sql.Tx) error {
			_, err := tx.ExecContext(c, "INSERT INTO config_lease_test VALUES(2)")
			return err
		})
	}); err != nil {
		t.Fatal(err)
	}
	close(resume)
	receiveConfig(t, done)
	var count, value int
	if err := db.QueryRow("SELECT count(*),max(value) FROM config_lease_test").Scan(&count, &value); err != nil || count != 1 || value != 2 {
		t.Fatal("successor evidence overwritten", count, value, err)
	}
}

func TestConfigObservationPartialAcquireCancelAndPanic(t *testing.T) {
	t.Setenv("DOB_CONFIG_OBSERVATION_PANELS", "all")
	db := adminTestDB(t)
	db.SetMaxOpenConns(5)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := sanaei.TryWithConfigReadLock(ctx, db, "partial", func(c context.Context) error { close(entered); return configGate(c, release) })
		done <- err
	}()
	awaitConfig(t, entered)
	// The writer owns C while waiting for O; cancellation must release C.
	c, stop := context.WithTimeout(ctx, 150*time.Millisecond)
	err := sanaei.WithConfigLock(c, db, "partial", func(context.Context) error { return errors.New("unexpected callback") })
	stop()
	if err == nil {
		t.Fatal("writer entered observation")
	}
	close(release)
	receiveConfig(t, done)
	var captured context.Context
	func() {
		defer func() {
			if recover() == nil {
				t.Error("callback panic disappeared")
			}
		}()
		_ = sanaei.WithConfigLock(ctx, db, "partial", func(c context.Context) error { captured = c; panic("test") })
	}()
	if sanaei.ConfigLockHeld(captured, db, "partial") {
		t.Fatal("escaped panic context kept authority")
	}
	got, err := sanaei.TryWithConfigLock(ctx, db, "partial", func(context.Context) error { return nil })
	if err != nil || !got {
		t.Fatal("partial/cancel/panic leaked session locks", got, err)
	}
}

func TestConfigObservationOptInAndPoolBudget(t *testing.T) {
	t.Setenv("DOB_CONFIG_OBSERVATION_PANELS", "selected")
	if sanaei.ConfigObservationEnabled("other") || !sanaei.ConfigObservationEnabled("selected") {
		t.Fatal("scope widened")
	}
	db := adminTestDB(t)
	db.SetMaxOpenConns(6)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Each proof pins one connection. Two observations publish on those same
	// sessions, leaving room for the bounded supporting SQL reads.
	t.Setenv("DOB_CONFIG_OBSERVATION_PANELS", "all")
	release := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(2)
	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			panel := fmt.Sprint("budget-", i)
			done <- sanaei.WithConfigLock(ctx, db, panel, func(c context.Context) error {
				if err := sanaei.ConfigProofOnly(c, db, panel); err != nil {
					return err
				}
				ready.Done()
				return configGate(c, release)
			})
		}(i)
	}
	ready.Wait()
	for i := 0; i < 2; i++ {
		panel := fmt.Sprint("budget-", i)
		got, err := sanaei.TryWithConfigReadLock(ctx, db, panel, func(c context.Context) error {
			return sanaei.WithConfigEvidenceTx(c, db, panel, func(tx *sql.Tx) error { _, e := tx.ExecContext(c, "SELECT 1"); return e })
		})
		if err != nil || !got {
			t.Fatal("bounded pool observation failed", got, err)
		}
	}
	close(release)
	receiveConfig(t, done)
	receiveConfig(t, done)
}
