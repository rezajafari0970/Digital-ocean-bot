package worker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRoleOwnershipRejectsDuplicateAndMixedProcesses(t *testing.T) {
	db := failureLedgerFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	control, err := AcquireRole(ctx, db, RoleControl)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	panels, err := AcquireRole(ctx, db, RolePanels)
	if err != nil {
		t.Fatal(err)
	}
	defer panels.Close()
	for _, role := range []Role{RoleAll, RoleControl, RolePanels} {
		l, err := AcquireRole(ctx, db, role)
		if l != nil {
			l.Close()
		}
		if !errors.Is(err, ErrRoleOwned) {
			t.Fatalf("%s: %v", role, err)
		}
	}
	if err := control.Check(ctx); err != nil {
		t.Fatal(err)
	}
	control.Close()
	// Failed all-role acquisition must not keep the acquired first lock.
	if l, err := AcquireRole(ctx, db, RoleAll); !errors.Is(err, ErrRoleOwned) {
		if l != nil {
			l.Close()
		}
		t.Fatal(err)
	}
	replacement, err := AcquireRole(ctx, db, RoleControl)
	if err != nil {
		t.Fatal("partial acquisition leaked", err)
	}
	replacement.Close()
	panels.Close()
	all, err := AcquireRole(ctx, db, RoleAll)
	if err != nil {
		t.Fatal(err)
	}
	defer all.Close()
	if err := all.Check(ctx); err != nil {
		t.Fatal(err)
	}
	// A lost role lock on a still-live connection must be detected.
	if _, err = all.conn.ExecContext(ctx, "SELECT pg_advisory_unlock_all()"); err != nil {
		t.Fatal(err)
	}
	if err = all.Check(ctx); err == nil {
		t.Fatal("ownership loss hidden")
	}
}
func TestRoleConnectionDeathStopsOwnershipProof(t *testing.T) {
	db := failureLedgerFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	l, err := AcquireRole(ctx, db, RoleControl)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if _, err = db.ExecContext(ctx, "SELECT pg_terminate_backend($1)", l.backend); err != nil {
		t.Fatal(err)
	}
	if err = l.Check(ctx); err == nil {
		t.Fatal("dead owner appears healthy")
	}
	replacement, err := AcquireRole(ctx, db, RoleControl)
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()
}
