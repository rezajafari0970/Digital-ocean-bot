package app

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// Execute the exact recovery SQL literals against PostgreSQL. This tests their
// state/metadata/CAS contract; it does not simulate provider/network admission.
func TestRecoveryDiagnosticsSQLPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	f, err := parser.ParseFile(token.NewFileSet(), "recovery.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	queries := map[string]string{}
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		q, e := strconv.Unquote(lit.Value)
		if e != nil || !strings.HasPrefix(q, "UPDATE operations SET ") {
			return true
		}
		switch {
		case strings.Contains(q, "resource_id=$3,state='verifying'"):
			queries["adoption"] = q
		case strings.Contains(q, "state='verifying',attempt=attempt+1"):
			queries["delete-reissue"] = q
		case strings.Contains(q, "state=$3,lock_version="):
			queries["observed-success"] = q
		}
		return true
	})
	if len(queries) != 3 {
		t.Fatalf("expected three recovery progress SQL paths, got %d", len(queries))
	}
	for name, q := range queries {
		t.Run(name, func(t *testing.T) {
			x := newDeleteFixture(t, db, "DELETING", "unknown")
			execBootstrap(t, db, "UPDATE operations SET error_code='transport',error_message='old error' WHERE id=$1", x.op)
			args := []any{x.op, x.item.AccountID, int64(0)}
			if name == "adoption" {
				args = []any{x.op, x.item.AccountID, x.item.ProviderID, int64(0)}
			}
			if name == "observed-success" {
				args = []any{x.op, x.item.AccountID, "succeeded", int64(0)}
			}
			result, e := db.Exec(q, args...)
			if e != nil {
				t.Fatal(e)
			}
			if e = requireRecoveryOperationUpdate(result, e); e != nil {
				t.Fatal(e)
			}
			var code, msg string
			if e = db.QueryRow("SELECT coalesce(error_code,''),coalesce(error_message,'') FROM operations WHERE id=$1", x.op).Scan(&code, &msg); e != nil {
				t.Fatal(e)
			}
			if code != "" || msg != "" {
				t.Fatalf("stale diagnostics after progress: %q %q", code, msg)
			}
			// The losing version cannot erase newer evidence.
			execBootstrap(t, db, "UPDATE operations SET state='unknown',error_code='capacity',error_message='winner evidence' WHERE id=$1", x.op)
			result, e = db.Exec(q, args...)
			if e != nil {
				t.Fatal(e)
			}
			if requireRecoveryOperationUpdate(result, nil) == nil {
				t.Fatal("obsolete version acknowledged")
			}
			if e = db.QueryRow("SELECT error_message FROM operations WHERE id=$1", x.op).Scan(&msg); e != nil || msg != "winner evidence" {
				t.Fatal("loser erased evidence", e)
			}
			if name == "observed-success" {
				args = []any{x.op, x.item.AccountID, "unknown", int64(1)}
				if _, e = db.Exec(q, args...); e != nil {
					t.Fatal(e)
				}
				if e = db.QueryRow("SELECT error_message FROM operations WHERE id=$1", x.op).Scan(&msg); e != nil || msg != "winner evidence" {
					t.Fatal("unknown outcome erased evidence", e)
				}
			}
		})
	}
}

func TestRecoveryDeleteCompletionDiagnosticsPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	x := newDeleteFixture(t, db, "DELETING", "unknown")
	execBootstrap(t, db, "UPDATE operations SET error_code='transport',error_message='old error' WHERE id=$1", x.op)
	c := Container{DB: db}
	if err := c.completeRecoveredDelete(context.Background(), x.op, x.item.AccountID, x.item.ProviderID, 1); err == nil {
		t.Fatal("stale version acknowledged")
	}
	var code, msg string
	if err := db.QueryRow("SELECT error_code,error_message FROM operations WHERE id=$1", x.op).Scan(&code, &msg); err != nil || code != "transport" || msg != "old error" {
		t.Fatal("conflict erased diagnostics", err)
	}
	if err := c.completeRecoveredDelete(context.Background(), x.op, x.item.AccountID, x.item.ProviderID, 0); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT coalesce(error_code,''),coalesce(error_message,'') FROM operations WHERE id=$1", x.op).Scan(&code, &msg); err != nil || code != "" || msg != "" {
		t.Fatalf("stale completion diagnostics: %q %q %v", code, msg, err)
	}
}
