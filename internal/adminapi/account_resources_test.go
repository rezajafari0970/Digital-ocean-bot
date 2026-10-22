package adminapi

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"sync"
	"sync/atomic"
	"testing"
)

func TestAccountResourceJournalConcurrencyAndRestart(t *testing.T) {
	db := adminTestDB(t)
	account := seedBuildAccount(t, db, "aws")
	j := providers.SQLAccountResources{DB: db, AccountID: account, Provider: "aws"}
	ctx := context.Background()
	var attempts atomic.Int32
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- j.WithLock(ctx, "eu-west-1:vpc-12345678", "fixture-network", func(s providers.AccountResourceSession) error {
				ok, e := s.BeginAttempt(ctx)
				if e != nil {
					return e
				}
				if ok {
					attempts.Add(1)
				}
				return nil
			})
		}()
	}
	wg.Wait()
	close(results)
	for e := range results {
		if e != nil {
			t.Fatal(e)
		}
	}
	if attempts.Load() != 1 {
		t.Fatal("duplicate network create admission", attempts.Load())
	}
	restarted := providers.SQLAccountResources{DB: db, AccountID: account, Provider: "aws"}
	e := restarted.WithLock(ctx, "eu-west-1:vpc-12345678", "fixture-network", func(s providers.AccountResourceSession) error {
		if !s.Record().Attempted || s.Record().Nonce == "" {
			return errors.New("intent lost")
		}
		if ok, e := s.BeginAttempt(ctx); ok || e != nil {
			return errors.New("unknown request replayed")
		}
		if e := s.Bind(ctx, "sg-12345678"); e != nil {
			return e
		}
		if e := s.Bind(ctx, "sg-87654321"); e == nil {
			return errors.New("binding changed")
		}
		ok, e := s.Missing(ctx)
		if e != nil {
			return e
		}
		if ok {
			return errors.New("first absence accepted")
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	sqlMust(t, db, "UPDATE provider_account_resources SET missing_since=now()-interval '3 minutes',last_missing_at=now()-interval '20 seconds',missing_count=2,last_attempt_at=now()-interval '4 minutes' WHERE account_id=$1", account)
	e = restarted.WithLock(ctx, "eu-west-1:vpc-12345678", "fixture-network", func(s providers.AccountResourceSession) error {
		stable, e := s.Missing(ctx)
		if e != nil {
			return e
		}
		if !stable {
			return errors.New("stable absence rejected")
		}
		return s.Finish(ctx)
	})
	if e != nil {
		t.Fatal(e)
	}
	active, e := j.List(ctx)
	if e != nil || len(active) != 0 {
		t.Fatal(active, e)
	}
	sqlMust(t, db, "UPDATE accounts SET enabled=false,deletion_requested_at=now() WHERE id=$1", account)
	if e = j.WithLock(ctx, "eu-west-1:vpc-87654321", "new-network", func(providers.AccountResourceSession) error { return nil }); e == nil {
		t.Fatal("deleting account admitted new network")
	}
}
