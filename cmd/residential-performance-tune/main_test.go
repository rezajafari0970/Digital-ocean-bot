package main

import (
	"os"
	"strings"
	"testing"
)

func TestReconcileRequiresProtectedDatabaseEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	input, err := os.CreateTemp(t.TempDir(), "request")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if _, err = input.WriteString(`{"action":"tune_cancel","request_id":"00000000-0000-0000-0000-000000000001"}`); err != nil {
		t.Fatal(err)
	}
	if _, err = input.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	before := os.Stdin
	os.Stdin = input
	defer func() { os.Stdin = before }()
	if err = run(false, true); err == nil || !strings.Contains(err.Error(), "protected database environment required") {
		t.Fatal("missing connection must fail before database access", err)
	}
}
