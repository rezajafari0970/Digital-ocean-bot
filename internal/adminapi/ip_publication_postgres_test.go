package adminapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/ipam"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"strings"
	"testing"
	"time"
)

func ipProofFixture(t *testing.T, db *sql.DB, s ipam.Store, e ipam.Endpoint, panel string) {
	t.Helper()
	sqlMust(t, db, "UPDATE ip_endpoints SET phase='NETWORK_READY',network_revision='fixture-network' WHERE id=$1", e.ID)
	sqlMust(t, db, `INSERT INTO ip_network_footprints(endpoint_id,revision,previous_revision,snapshot_digest,plan_digest,state) VALUES($1,'fixture-network','before',$2,$2,'APPLIED')`, e.ID, strings.Repeat("b", 64))
	now := time.Now().UTC()
	var revision int64
	if e := db.QueryRow("SELECT revision FROM residential_routing_control").Scan(&revision); e != nil {
		t.Fatal(e)
	}
	p := ipam.HealthProof{EndpointID: e.ID, EndpointRevision: e.Revision, NetworkRevision: "fixture-network", PanelID: panel, ConfigRevision: revision, Port: 443, RouteClass: "DIRECT",
		ProviderAttached: true, GuestConfigured: true, ProtocolOK: true, RouteIsolationOK: true, UserNetworkOK: true, FallbackOK: true, ObservedAt: now, ExpiresAt: now.Add(time.Minute), EvidenceDigest: strings.Repeat("c", 64)}
	if err := s.RecordProof(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}
func ipPublicationFixture(t *testing.T, db *sql.DB) (ipam.Store, ipam.Endpoint, ipam.Endpoint, ipam.Publication) {
	t.Helper()
	ctx := context.Background()
	account, droplet, panel := seedPanel(t, db, "http://fixture.invalid")
	sqlMust(t, db, "UPDATE droplets SET provider_resource_id='server-1' WHERE id=$1", droplet)
	var revision int64
	if e := db.QueryRow("SELECT revision FROM residential_routing_control").Scan(&revision); e != nil {
		t.Fatal(e)
	}
	sqlMust(t, db, "INSERT INTO panel_routing_state(panel_id,revision,state,verified_at) VALUES($1,$2,'APPLIED',now())", panel, revision)
	p := ipPolicy(6)
	p.Desired = ipPtr(2)
	p.PerRun = ipPtr(2)
	p.MaxConcurrent = ipPtr(2)
	s, _, ids := ipApprovedBatch(t, db, account, 6, p)
	for i, id := range ids {
		sqlMust(t, db, "UPDATE ip_mutation_lanes SET next_dispatch_at=NULL WHERE account_id=$1", account) // fixture elapsed time
		op, e := s.Dispatch(ctx, id, ipGeneration(t, db, account))
		if e != nil {
			t.Fatal(e)
		}
		if e = s.RecordResponse(ctx, id, "response", ipam.DispatchReceipt{DispatchID: op.DispatchID, RequestDigest: op.RequestDigest, Outcome: "ACQUIRED", Contract: ipam.AcquisitionContract("digitalocean", "reserved", op.Family), HTTPStatus: 201, ResourceID: fmt.Sprint("resource-", i)}); e != nil {
			t.Fatal(e)
		}
	}
	ipAllow(t, db, account)
	inv := ipam.InventoryStore{DB: db}
	tick, e := inv.Begin(ctx, account)
	if e != nil {
		t.Fatal(e)
	}
	o := providers.IPObservation{Complete: true, CredentialGeneration: ipGeneration(t, db, account), ObservedAt: time.Now().UTC(), Quotas: []providers.IPQuota{}, Resources: []providers.IPResource{
		{ID: "resource-0", Family: 6, Kind: "reserved", Prefix: "2001:db8:100:1::/64", ServerID: "server-1", Region: "region-1", Protected: true},
		{ID: "resource-1", Family: 6, Kind: "reserved", Prefix: "2001:db8:100:2::/64", ServerID: "server-1", Region: "region-1", Protected: true}}}
	if e = inv.Finish(ctx, tick, &o, "", 0, 0); e != nil {
		t.Fatal(e)
	}
	allocs := []string{}
	for _, id := range ids {
		a, err := s.ConfirmAcquisition(ctx, id, tick.RequestID)
		if err != nil {
			t.Fatal(err)
		}
		allocs = append(allocs, a)
	}
	old, e := s.SelectEndpoint(ctx, allocs[0], "2001:db8:100:1::100")
	if e != nil {
		t.Fatal(e)
	}
	replacement, e := s.SelectEndpoint(ctx, allocs[1], "2001:db8:100:2::100")
	if e != nil {
		t.Fatal(e)
	}
	ipProofFixture(t, db, s, old, panel)
	ipProofFixture(t, db, s, replacement, panel)
	until := time.Now().Add(30 * time.Minute)
	pub := ipam.Publication{AccountID: account, ServerID: "server-1", PanelID: panel, ClientID: "fixture-client", URI: "vless://fixture-client@8.8.8.8:443?security=reality&sni=example.com#keep%20bytes", RouteClass: "DIRECT", ConfigRevision: revision, ClientValidUntil: &until}
	return s, old, replacement, pub
}
func TestIPLifecyclePublicationLeasesCommitAndKeepURI(t *testing.T) {
	db := adminTestDB(t)
	s, old, _, pub := ipPublicationFixture(t, db)
	ctx := context.Background()
	out, e := s.Publish(ctx, pub)
	if e != nil || len(out) != 2 {
		t.Fatal(out, e)
	}
	if !strings.Contains(out[0].URI, "@[2001:db8:100:") || !strings.HasSuffix(out[0].URI, "?security=reality&sni=example.com#keep%20bytes") {
		t.Fatal(out)
	}
	var until time.Time
	var count int
	if e = db.QueryRow("SELECT count(*) FROM ip_publication_leases").Scan(&count); e != nil || count != 2 {
		t.Fatal("returned before durable lease", count, e)
	}
	if e = db.QueryRow("SELECT valid_until FROM ip_publication_leases WHERE endpoint_id=$1", old.ID).Scan(&until); e != nil || until.Sub(*pub.ClientValidUntil) > time.Microsecond || pub.ClientValidUntil.Sub(until) > time.Microsecond {
		t.Fatal("visibility confused with actual client validity", until, e)
	}
	// A shorter publication TTL must not shorten an existing lease.
	short := time.Now().Add(time.Minute)
	pub.ClientValidUntil = &short
	if _, e = s.Publish(ctx, pub); e != nil {
		t.Fatal(e)
	}
	var again time.Time
	if e = db.QueryRow("SELECT valid_until FROM ip_publication_leases WHERE endpoint_id=$1", old.ID).Scan(&again); e != nil || !again.Equal(until) {
		t.Fatal("lease shortened", e)
	}
	pub.ClientValidUntil = nil
	if out, e = s.Publish(ctx, pub); e != nil || len(out) != 0 {
		t.Fatal("unfunded unbounded publication", out, e)
	}
}
func TestIPLifecyclePublicationFailureAndProofScope(t *testing.T) {
	db := adminTestDB(t)
	s, _, _, pub := ipPublicationFixture(t, db)
	ctx := context.Background()
	sqlMust(t, db, `CREATE FUNCTION ip_fixture_reject_lease() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture'; END $$; CREATE TRIGGER ip_fixture_lease BEFORE INSERT ON ip_publication_leases FOR EACH ROW EXECUTE FUNCTION ip_fixture_reject_lease()`)
	if out, e := s.Publish(ctx, pub); e == nil || len(out) > 0 {
		t.Fatal("uncommitted configuration escaped", out, e)
	}
	sqlMust(t, db, "DROP TRIGGER ip_fixture_lease ON ip_publication_leases")
	pub.RouteClass = "RESIDENTIAL"
	if out, e := s.Publish(ctx, pub); e != nil || len(out) != 0 {
		t.Fatal("direct proof reused for residential", out, e)
	}
	pub.RouteClass = "DIRECT"
	originalRevision := pub.ConfigRevision
	pub.ConfigRevision++
	if out, e := s.Publish(ctx, pub); e != nil || len(out) != 0 {
		t.Fatal("stale config proof", out, e)
	}
	pub.ConfigRevision = originalRevision
	sqlMust(t, db, "UPDATE ip_endpoint_proofs SET expires_at=now()-interval '1 second',observed_at=now()-interval '1 minute'")
	if out, e := s.Publish(ctx, pub); e != nil || len(out) != 0 {
		t.Fatal("expired health proof", out, e)
	}
}
func TestIPLifecycleDrainRetainsLeasesAndFailedReplacement(t *testing.T) {
	db := adminTestDB(t)
	s, old, replacement, pub := ipPublicationFixture(t, db)
	ctx := context.Background()
	if _, e := s.Publish(ctx, pub); e != nil {
		t.Fatal(e)
	}
	if e := s.BeginDrain(ctx, old.ID, replacement.ID); e != nil {
		t.Fatal(e)
	}
	if e := s.Retire(ctx, old.ID); !errors.Is(e, ipam.ErrProtected) {
		t.Fatal("active client lease retired", e)
	}
	if out, e := s.Publish(ctx, pub); e != nil || len(out) != 1 || out[0].EndpointID != replacement.ID {
		t.Fatal("draining endpoint republished", out, e)
	}
	sqlMust(t, db, "UPDATE ip_publication_leases SET valid_until=NULL WHERE endpoint_id=$1", old.ID)
	if e := s.Retire(ctx, old.ID); !errors.Is(e, ipam.ErrProtected) {
		t.Fatal("unbounded lease retired", e)
	}
	// Test-only verified migration receipt; no production migration endpoint exists yet.
	sqlMust(t, db, "UPDATE ip_publication_leases SET migrated_at=now(),migration_receipt='fixture-client-ack' WHERE endpoint_id=$1", old.ID)
	sqlMust(t, db, "UPDATE ip_endpoint_proofs SET expires_at=now()-interval '1 second',observed_at=now()-interval '1 minute' WHERE endpoint_id=$1", replacement.ID)
	if e := s.Retire(ctx, old.ID); !errors.Is(e, ipam.ErrProtected) {
		t.Fatal("failed replacement retired old endpoint", e)
	}
	// Refresh verified replacement without changing the old lease or dropping its parent.
	sqlMust(t, db, "UPDATE ip_endpoint_proofs SET observed_at=now(),expires_at=now()+interval '30 seconds' WHERE endpoint_id=$1", replacement.ID)
	if e := s.Retire(ctx, old.ID); e != nil {
		t.Fatal(e)
	}
	var state string
	if e := db.QueryRow("SELECT state FROM ip_allocations WHERE id=$1", old.AllocationID).Scan(&state); e != nil || state != "OWNED" {
		t.Fatal("endpoint retirement released prefix", state, e)
	}
}
