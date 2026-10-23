package app

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"testing"
	"time"
)

type versionedCatalogDriver struct {
	economyDriver
	version string
}

func (d *versionedCatalogDriver) CatalogCacheVersion() string { return d.version }
func TestVersionedCatalogRefreshIndependentOfEconomy(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	execBootstrap(t, db, "UPDATE proxy_economy_policy SET enabled=false")
	var id string
	if err := db.QueryRow("INSERT INTO accounts(id,provider,name,secret_ref) VALUES(gen_random_uuid(),'ovh','catalog-version','unused') RETURNING id::text").Scan(&id); err != nil {
		t.Fatal(err)
	}
	c := Container{DB: db, Economy: &network.EconomyController{DB: db}}
	d := &versionedCatalogDriver{version: "v1"}
	rt := AccountRuntime{Config: AccountConfig{ID: id, Provider: "ovh", SecretRef: "unused"}, Driver: d}
	for i := 0; i < 2; i++ {
		if _, err := c.CachedProviderCatalog(ctx, rt); err != nil {
			t.Fatal(err)
		}
	}
	if d.catalogCalls.Load() != 1 {
		t.Fatalf("same version refetched %d times", d.catalogCalls.Load())
	}
	d.version = "v2"
	if _, err := c.CachedProviderCatalog(ctx, rt); err != nil {
		t.Fatal(err)
	}
	if d.catalogCalls.Load() != 2 {
		t.Fatal("schema change reused old catalog")
	}
	if _, err := db.Exec("INSERT INTO secrets(id,account_id,kind,ciphertext,nonce,key_version) VALUES('unused',$1,'token','a','b',1)", id); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CachedProviderCatalog(ctx, rt); err != nil {
		t.Fatal(err)
	}
	if d.catalogCalls.Load() != 3 {
		t.Fatal("credential change reused catalog")
	}
	if _, err := db.Exec("UPDATE provider_catalog_cache SET refreshed_at=now()-interval '7 hours' WHERE account_id=$1", id); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CachedProviderCatalog(ctx, rt); err != nil {
		t.Fatal(err)
	}
	if d.catalogCalls.Load() != 4 {
		t.Fatal("expired catalog not refreshed")
	}
}
func TestVersionedCatalogHealthDoesNotFetch(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	execBootstrap(t, db, "UPDATE proxy_economy_policy SET enabled=false")
	var id string
	if err := db.QueryRow("INSERT INTO accounts(id,provider,name,secret_ref) VALUES(gen_random_uuid(),'ovh','catalog-health','unused') RETURNING id::text").Scan(&id); err != nil {
		t.Fatal(err)
	}
	old, _ := json.Marshal(providers.Observation{Catalog: providers.Catalog{Plans: []providers.Plan{{ID: "obsolete"}}}})
	if _, err := db.Exec("INSERT INTO provider_snapshots(id,account_id,provider,data,canonical,created_at) VALUES(gen_random_uuid(),$1,'ovh','{}',$2,now())", id, old); err != nil {
		t.Fatal(err)
	}
	c := Container{DB: db, Economy: &network.EconomyController{DB: db}}
	d := &versionedCatalogDriver{version: "v1"}
	d.fail.Store(true)
	rt := AccountRuntime{Config: AccountConfig{ID: id, Provider: "ovh", SecretRef: "unused"}, Driver: d}
	obs, err := c.ObserveProvider(ctx, rt)
	if err != nil || obs.Capacity.ComputeLimit != 7 || d.catalogCalls.Load() != 0 {
		t.Fatalf("optional catalog blocked health: %+v %v", obs, err)
	}
	if len(obs.Catalog.Plans) != 0 {
		t.Fatal("fresh snapshot resurrected obsolete catalog")
	}
	d.fail.Store(false)
	if _, err = c.CachedProviderCatalog(ctx, rt); err != nil {
		t.Fatal(err)
	}
	obs, err = c.ObserveProvider(ctx, rt)
	if err != nil || len(obs.Catalog.Plans) != 1 || obs.Catalog.Plans[0].ID != "p" || d.catalogCalls.Load() != 1 {
		t.Fatalf("refreshed catalog not adopted: %+v %v", obs, err)
	}
	var raw []byte
	if err = db.QueryRow("SELECT catalog FROM provider_catalog_cache WHERE account_id=$1", id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	json.Unmarshal(raw, &meta)
	stamp, e := time.Parse(time.RFC3339Nano, meta["FetchedAt"].(string))
	if e != nil || time.Since(stamp) > time.Minute {
		t.Fatal("actual fetch timestamp missing")
	}
}
