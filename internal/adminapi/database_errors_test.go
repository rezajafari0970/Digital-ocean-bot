package adminapi

import (
	"errors"
	"fmt"
	"github.com/lib/pq"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDatabaseErrorsDoNotLeakOrMisclassify(t *testing.T) {
	for _, code := range []pq.ErrorCode{"XX001", "XX002", "08006", "42501"} {
		w := httptest.NewRecorder()
		err := fmt.Errorf("read: %w", &pq.Error{Code: code, Message: "private table or secret"})
		writeDatabaseReadError(w, err, "accounts_query_failed")
		corruption := code == "XX001" || code == "XX002"
		if (w.Code == 503) != corruption || strings.Contains(w.Body.String(), "private") {
			t.Fatalf("%s: %s", code, w.Body.String())
		}
		if corruption && w.Header().Get("Retry-After") != "30" {
			t.Fatal("missing retry bound")
		}
	}
	w := httptest.NewRecorder()
	writeDatabaseReadError(w, errors.New("private SQL"), "accounts_query_failed")
	if w.Code != 500 || strings.Contains(w.Body.String(), "private") {
		t.Fatal(w.Body.String())
	}
}
