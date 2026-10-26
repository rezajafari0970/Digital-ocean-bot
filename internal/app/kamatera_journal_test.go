package app

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"testing"
)

func TestKamateraResourceIntentSurvivesCrashAndHasUniqueBinding(t *testing.T) {
	db := installerBootstrapDB(t)
	f := newBootstrapFixture(t, db)
	other := newBootstrapFixture(t, db)
	ctx := context.Background()
	for _, id := range []string{f.d.AccountID, other.d.AccountID} {
		execBootstrap(t, db, "UPDATE accounts SET provider='kamatera' WHERE id=$1", id)
	}
	j := providers.SQLAccountResources{DB: db, AccountID: f.d.AccountID, Provider: "kamatera"}
	crash := errors.New("fixture crash")
	spec := `{"region":"EU","plan":"fixture","image":"fixture"}`
	e := j.WithLock(ctx, "server:fixture", "name", func(s providers.AccountResourceSession) error {
		a := s.(providers.AsyncResourceSession)
		if e := a.PrepareIntent(ctx, spec); e != nil {
			return e
		}
		ok, e := s.BeginAttempt(ctx)
		if e != nil || !ok {
			t.Fatal("claim", ok, e)
		}
		if e = a.RecordOperation(ctx, "12345"); e != nil {
			return e
		}
		return crash
	})
	if !errors.Is(e, crash) {
		t.Fatal(e)
	}
	e = j.WithLock(ctx, "server:fixture", "name", func(s providers.AccountResourceSession) error {
		r := s.Record()
		if !r.Attempted || r.OperationID != "12345" || r.Intent == "{}" {
			t.Fatal("crash erased durable intent", r)
		}
		a := s.(providers.AsyncResourceSession)
		if e := a.PrepareIntent(ctx, `{"region":"US"}`); e == nil {
			t.Fatal("spec changed after dispatch")
		}
		if e := s.Bind(ctx, "11111111-1111-4111-8111-111111111111"); e != nil {
			return e
		}
		ok, e := a.BeginDeletion(ctx)
		if e != nil || !ok {
			t.Fatal("delete claim", ok, e)
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	e = j.WithLock(ctx, "server:fixture", "name", func(s providers.AccountResourceSession) error {
		ok, e := s.(providers.AsyncResourceSession).BeginDeletion(ctx)
		if e != nil || ok || !s.Record().DeleteAttempted {
			t.Fatal("duplicate delete claim", ok, e)
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	otherJournal := providers.SQLAccountResources{DB: db, AccountID: other.d.AccountID, Provider: "kamatera"}
	e = otherJournal.WithLock(ctx, "server:other", "other-name", func(s providers.AccountResourceSession) error {
		return s.Bind(ctx, "11111111-1111-4111-8111-111111111111")
	})
	if e == nil {
		t.Fatal("resource bound to two accounts")
	}
}
