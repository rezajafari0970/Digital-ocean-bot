package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func TestKamateraRegionalAdmissionIsAtomicAndAccountScoped(t *testing.T) {
	db := installerBootstrapDB(t)
	f, other := newBootstrapFixture(t, db), newBootstrapFixture(t, db)
	ctx := context.Background()
	for _, id := range []string{f.d.AccountID, other.d.AccountID} {
		execBootstrap(t, db, "UPDATE accounts SET provider='kamatera' WHERE id=$1", id)
	}
	j := providers.SQLAccountResources{DB: db, AccountID: f.d.AccountID, Provider: "kamatera"}
	claim := func(owner providers.SQLAccountResources, scope, region string) error {
		return owner.WithLock(ctx, "server:"+scope, scope, func(s providers.AccountResourceSession) error {
			intent, _ := json.Marshal(map[string]string{"region": region, "plan": "fixture", "image": "fixture"})
			ok, err := s.(providers.RegionalCreateSession).BeginRegionalCreate(ctx, string(intent))
			if err == nil && !ok {
				return errors.New("unexpected missing claim")
			}
			return err
		})
	}
	const n = 6
	start := make(chan struct{})
	type result struct {
		scope string
		err   error
	}
	results := make(chan result, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			scope := fmt.Sprintf("candidate-%d", i)
			results <- result{scope, claim(j, scope, "US-MI2")}
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	winner, blocked := "", []string{}
	for r := range results {
		if r.err == nil {
			if winner != "" {
				t.Fatal("two simultaneous regional claims")
			}
			winner = r.scope
		} else if errors.Is(r.err, providers.ErrRegionalCreateUnresolved) {
			blocked = append(blocked, r.scope)
		} else {
			t.Fatal(r.err)
		}
	}
	if winner == "" || len(blocked) != n-1 {
		t.Fatalf("claims=%s blocked=%v", winner, blocked)
	}
	// Time alone must never prove that the first billable request did not run.
	execBootstrap(t, db, "UPDATE provider_account_resources SET last_attempt_at=now()-interval '1 day' WHERE account_id=$1 AND scope=$2", f.d.AccountID, "server:"+winner)
	if err := claim(j, blocked[0], "US-MI2"); !errors.Is(err, providers.ErrRegionalCreateUnresolved) {
		t.Fatalf("age reopened ambiguous create: %v", err)
	}
	if err := claim(j, blocked[0], "US-CH"); err != nil {
		t.Fatal("undispatched region fallback", err)
	}
	otherJournal := providers.SQLAccountResources{DB: db, AccountID: other.d.AccountID, Provider: "kamatera"}
	if err := claim(otherJournal, "independent-account", "US-MI2"); err != nil {
		t.Fatal("cross-account block", err)
	}
	err := j.WithLock(ctx, "server:"+winner, winner, func(s providers.AccountResourceSession) error {
		return s.(providers.AsyncResourceSession).RecordOperation(ctx, "12345")
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := claim(j, blocked[1], "US-MI2"); err != nil {
		t.Fatal("acknowledged task blocked parallel provisioning", err)
	}
	var claims int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM provider_account_resources WHERE account_id=$1 AND last_attempt_at IS NOT NULL", f.d.AccountID).Scan(&claims); err != nil || claims != 3 {
		t.Fatal("unexpected dispatch claims", claims, err)
	}
}

func TestKamateraRegionalAdmissionPreservesCancellationAndUnknownScope(t *testing.T) {
	db := installerBootstrapDB(t)
	f := newBootstrapFixture(t, db)
	ctx := context.Background()
	execBootstrap(t, db, "UPDATE accounts SET provider='kamatera' WHERE id=$1", f.d.AccountID)
	j := providers.SQLAccountResources{DB: db, AccountID: f.d.AccountID, Provider: "kamatera"}
	if err := j.WithLock(ctx, "server:legacy", "legacy", func(s providers.AccountResourceSession) error { _, err := s.BeginAttempt(ctx); return err }); err != nil {
		t.Fatal(err)
	}
	if err := j.WithLock(ctx, "server:new", "new", func(s providers.AccountResourceSession) error {
		_, err := s.(providers.RegionalCreateSession).BeginRegionalCreate(ctx, `{"region":"US-CH"}`)
		return err
	}); !errors.Is(err, providers.ErrRegionalCreateUnresolved) {
		t.Fatal("unknown legacy region ignored", err)
	}
	if err := j.WithLock(ctx, "server:legacy", "legacy", func(s providers.AccountResourceSession) error { return s.Finish(ctx) }); err != nil {
		t.Fatal(err)
	}
	if err := j.WithLock(ctx, "server:new", "new", func(s providers.AccountResourceSession) error {
		execBootstrap(t, db, "UPDATE accounts SET enabled=false WHERE id=$1", f.d.AccountID)
		ok, err := s.(providers.RegionalCreateSession).BeginRegionalCreate(ctx, `{"region":"US-CH"}`)
		if err == nil || ok {
			return errors.New("disabled account claimed create")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var attempted bool
	if err := db.QueryRowContext(ctx, "SELECT last_attempt_at IS NOT NULL FROM provider_account_resources WHERE account_id=$1 AND scope='server:new'", f.d.AccountID).Scan(&attempted); err != nil || attempted {
		t.Fatal("cancellation changed journal", attempted, err)
	}
}
