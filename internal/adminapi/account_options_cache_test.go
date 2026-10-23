package adminapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestAccountOptionsUsesActualCatalogFetch(t *testing.T) {
	db := adminTestDB(t)
	id := seedBuildAccount(t, db, "ovh")
	putBuildSnapshot(t, db, id, "active", true, 3, 0)
	sqlMust(t, db, `INSERT INTO provider_catalog_cache(account_id,catalog,refreshed_at,scope_revision)
 VALUES($1,jsonb_build_object('CacheVersion','fixture-v2','FetchedAt',now()-interval '1 minute','Plans',jsonb_build_array(jsonb_build_object('ID','complete-plan')),'Pricing',jsonb_build_object('Status','available','Currency','EUR')),now()-interval '1 minute','ovh/fixture/missing/catalog:fixture-v2')`, id)
	req := httptest.NewRequest("GET", "/", nil)
	req.SetPathValue("id", id)
	w := httptest.NewRecorder()
	s := Server{DB: db}
	s.accountOptions(w, req)
	var out map[string]json.RawMessage
	if json.Unmarshal(w.Body.Bytes(), &out) != nil {
		t.Fatal(w.Body.String())
	}
	var source string
	json.Unmarshal(out["source"], &source)
	if source != "catalog_cache" {
		t.Fatalf("snapshot wall clock hid actual catalog: %s", w.Body.String())
	}
	var plans []map[string]any
	json.Unmarshal(out["plans"], &plans)
	if len(plans) != 1 || plans[0]["ID"] != "complete-plan" {
		t.Fatal("fresh catalog missing")
	}
	sqlMust(t, db, "INSERT INTO secrets(id,account_id,kind,ciphertext,nonce,key_version) VALUES('fixture',$1,'token','a','b',1)", id)
	w = httptest.NewRecorder()
	s.accountOptions(w, req)
	json.Unmarshal(w.Body.Bytes(), &out)
	json.Unmarshal(out["source"], &source)
	if source == "catalog_cache" {
		t.Fatal("credential rotation reused old scoped catalog")
	}
}
