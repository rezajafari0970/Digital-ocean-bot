package migrate

import "testing"

func TestTransactionEscapeLexing(t *testing.T) {
	for _, sql := range []string{"COMMIT", "BEGIN;", "-- comment\nROLLBACK", "SELECT 1; END;", "START TRANSACTION", "PREPARE TRANSACTION 'id'", "SELECT '\\'; COMMIT; -- '"} {
		if rejectTransactionEscape(sql) == nil {
			t.Fatal("accepted", sql)
		}
	}
	for _, sql := range []string{"SELECT 'COMMIT';", "SELECT E'\\'COMMIT';", "DO $body$ BEGIN RAISE NOTICE 'COMMIT'; END $body$;", "/* /* COMMIT */ nested */ SELECT 1;", "SELECT \"COMMIT\" FROM records;"} {
		if err := rejectTransactionEscape(sql); err != nil {
			t.Fatal(sql, err)
		}
	}
}
