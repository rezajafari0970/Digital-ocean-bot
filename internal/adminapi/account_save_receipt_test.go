package adminapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestAccountSaveReturnsCommittedRuleReceipt(t *testing.T) {
	db := adminTestDB(t)
	a, _ := ruleFixture(t, db, "ovh", 1)
	for _, apply := range []bool{false, true} {
		w := ruleSave(t, db, a, apply, 10, 59580, 60360)
		var got struct {
			OK      bool `json:"ok"`
			Account struct {
				ID      string         `json:"id"`
				Desired int            `json:"desired_server_count"`
				Rules   map[string]any `json:"rule_application"`
			} `json:"account"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if !got.OK || got.Account.ID != a || got.Account.Desired != 10 || got.Account.Rules["apply_to_existing"] != apply {
			t.Fatal("saved transaction lacks exact receipt", w.Body.String())
		}
		var raw []byte
		if err := db.QueryRow("SELECT to_jsonb(p)-'account_id' FROM account_rule_application p WHERE account_id=$1", a).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var expected map[string]any
		json.Unmarshal(raw, &expected)
		e, _ := json.Marshal(expected)
		g, _ := json.Marshal(got.Account.Rules)
		if string(e) != string(g) {
			t.Fatal("receipt differs from committed state", string(e), string(g))
		}
	}
}

func TestAccountListNullInventoryDoesNotBreakSavedStatus(t *testing.T) {
	db := adminTestDB(t)
	a, _ := ruleFixture(t, db, "ovh", 1)
	ruleSave(t, db, a, false, 10, 59580, 60360)
	for _, inventory := range []string{"null", "[]"} {
		sqlMust(t, db, `UPDATE provider_snapshots SET canonical=jsonb_set(canonical,'{Inventory,Servers}',$2::jsonb) WHERE account_id=$1`, a, inventory)
		w := httptest.NewRecorder()
		(&Server{DB: db}).accounts(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != 200 {
			t.Fatal("unavailable regions broke account read", inventory, w.Code, w.Body.String())
		}
		var rows []map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0]["desired_server_count"] != float64(10) || len(rows[0]["built_regions"].([]any)) != 0 {
			t.Fatal(rows)
		}
	}
}
