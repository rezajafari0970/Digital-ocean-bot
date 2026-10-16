package residentialsync

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"testing"
	"time"
)

type noProofSecrets struct{}

func (noProofSecrets) Get(context.Context, string, string) ([]byte, error) {
	return nil, errors.New("unexpected credentials read")
}
func (noProofSecrets) GetResidential(context.Context, string, string) ([]byte, error) {
	return nil, errors.New("unexpected residential credential read")
}

func TestProofRechecksDueAfterQueuedOwnership(t *testing.T) {
	db := trialTestDB(t)
	id := trialUUID()
	sqlMustTrial(t, db, "UPDATE residential_routing_control SET enabled=true,fleet=true")
	sqlMustTrial(t, db, "INSERT INTO accounts(id,name,provider,secret_ref) VALUES($1,'fixture','digitalocean','fixture')", id)
	sqlMustTrial(t, db, "INSERT INTO droplets(id,account_id,provider_resource_id,state) VALUES($1,$1,'fixture','READY')", id)
	sqlMustTrial(t, db, "INSERT INTO deployment_profiles(id,account_id,name,config) VALUES($1,$1,'fixture','{}')", id)
	sqlMustTrial(t, db, "INSERT INTO deployments(id,account_id,profile_id,droplet_id,state,current_step) VALUES($1,$1,$1,$1,'PANEL_COMPLETE','done')", id)
	sqlMustTrial(t, db, "INSERT INTO panel_instances(id,account_id,droplet_id,driver,base_url,auth_secret_ref) VALUES($1,$1,$1,'sanaei-3x-ui','http://fixture','fixture')", id)
	sqlMustTrial(t, db, "INSERT INTO panel_routing_state(panel_id,revision,state,verified_at,next_check_at) SELECT $1,revision,'APPLIED',now()-interval '70 seconds',now()-interval '1 second' FROM residential_routing_control", id)
	sqlMustTrial(t, db, "INSERT INTO output_config_snapshots(panel_id,uri,client_id) VALUES($1,'vless://fixture','fixture')", id)
	svc := Service{DB: db, Secrets: noProofSecrets{}, Runtimes: &sanaei.RuntimeManager{}}
	coverage, e := svc.Coverage(context.Background())
	if e != nil || coverage.Eligible != 1 || coverage.Applied != 1 || coverage.Fresh != 0 || coverage.RawWithheld != 1 {
		t.Fatal("stale APPLIED falsely counted fresh", coverage, e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	held, release := make(chan struct{}), make(chan struct{})
	owner := make(chan error, 1)
	go func() {
		owner <- sanaei.WithConfigLock(ctx, db, id, func(context.Context) error {
			close(held)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	<-held
	done := make(chan error, 1)
	go func() { done <- svc.ReconcileDuePanel(ctx, readyworker.Panel{ID: id}, true) }()
	for {
		var queued bool
		e := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND NOT granted AND classid::bigint=((hashtextextended($1,0)>>32)&4294967295) AND objid::bigint=(hashtextextended($1,0)&4294967295))`, "panel-config:"+id).Scan(&queued)
		if e != nil {
			t.Fatal(e)
		}
		if queued {
			break
		}
		time.Sleep(time.Millisecond * 5)
	}
	sqlMustTrial(t, db, "UPDATE panel_routing_state SET verified_at=now(),next_check_at=now()+interval '20 seconds' WHERE panel_id=$1", id)
	var before time.Time
	if e = db.QueryRow("SELECT verified_at FROM panel_routing_state WHERE panel_id=$1", id).Scan(&before); e != nil {
		t.Fatal(e)
	}
	close(release)
	if e = <-owner; e != nil {
		t.Fatal(e)
	}
	if e = <-done; e != nil {
		t.Fatal("obsolete waiter did not skip", e)
	}
	var after time.Time
	if e = db.QueryRow("SELECT verified_at FROM panel_routing_state WHERE panel_id=$1", id).Scan(&after); e != nil || !after.Equal(before) {
		t.Fatal("obsolete attempt refreshed evidence", e)
	}
	coverage, e = svc.Coverage(ctx)
	if e != nil || coverage.Fresh != 1 || coverage.RawWithheld != 0 {
		t.Fatal(coverage, e)
	}
	// Serving/retirement transitions are rechecked with the same selection predicate.
	sqlMustTrial(t, db, "UPDATE panel_routing_state SET next_check_at=now()-interval '1 second' WHERE panel_id=$1", id)
	sqlMustTrial(t, db, "UPDATE droplets SET state='PENDING_DELETION' WHERE id=$1", id)
	if e = svc.ReconcileDuePanel(ctx, readyworker.Panel{ID: id}, true); e != nil {
		t.Fatal("retired server not skipped", e)
	}
	if e = sanaei.WithConfigLock(ctx, db, id, func(c context.Context) error {
		serving := false
		due, e := svc.dueWhileOwned(c, id, &serving)
		if e != nil {
			return e
		}
		if !due {
			return errors.New("retirement starved")
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
}
