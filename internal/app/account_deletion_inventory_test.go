package app

import (
	"context"
	"errors"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

type deletionKeyRead struct {
	keys []providers.SSHKey
	err  error
}
type deletionKeyInventoryFixture struct {
	providers.Driver
	providers.SSHKeyDriver
	steps          []deletionKeyRead
	reads, deletes int
	deleteErr      error
}

func (d *deletionKeyInventoryFixture) Capabilities() providers.Capabilities {
	return providers.Capabilities{}
}
func (d *deletionKeyInventoryFixture) ListSSHKeys(context.Context) ([]providers.SSHKey, error) {
	i := d.reads
	d.reads++
	if i >= len(d.steps) {
		return nil, errors.New("unexpected inventory read")
	}
	return d.steps[i].keys, d.steps[i].err
}
func (d *deletionKeyInventoryFixture) DeleteSSHKey(_ context.Context, id string) error {
	if id != "owned-key" {
		return errors.New("foreign key deletion")
	}
	d.deletes++
	return d.deleteErr
}

func TestAccountSSHDeletionRequiresValidAbsenceEvidence(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c := Container{DB: db}
	other := newBootstrapFixture(t, db)
	execBootstrap(t, db, "INSERT INTO account_deletion_keys(account_id,provider_key_id,state) VALUES($1,'other-key','RUNNING')", other.d.AccountID)
	for _, scenario := range []string{"initial-invalid", "confirmation-invalid", "confirmation-error", "still-present", "confirmed-absent", "lost-delete-response"} {
		t.Run(scenario, func(t *testing.T) {
			f := newBootstrapFixture(t, db)
			execBootstrap(t, db, "UPDATE deployments SET profile_snapshot=jsonb_set(profile_snapshot,'{ssh_provider_key_id}',to_jsonb($2::text)) WHERE id=$1", f.d.ID, "owned-key")
			owned := providers.SSHKey{ID: "owned-key", Name: "dob-" + f.d.ID}
			foreign := providers.SSHKey{ID: "foreign", Name: "manual"}
			fake := &deletionKeyInventoryFixture{steps: []deletionKeyRead{{keys: []providers.SSHKey{owned, foreign}}, {keys: []providers.SSHKey{foreign}}}}
			wantError := true
			wantDeletes := 1
			switch scenario {
			case "initial-invalid":
				fake.steps[0].keys = []providers.SSHKey{owned, {Name: "missing-id"}}
				wantDeletes = 0
			case "confirmation-invalid":
				fake.steps[1].keys = []providers.SSHKey{{Name: "missing-id"}}
			case "confirmation-error":
				fake.steps[1].err = errors.New("fixture read failure")
			case "still-present":
				fake.steps[1].keys = []providers.SSHKey{owned, foreign}
			case "confirmed-absent":
				wantError = false
			case "lost-delete-response":
				wantError = false
				fake.deleteErr = context.DeadlineExceeded
			}
			rt := AccountRuntime{Driver: fake, Gate: network.AccountGate{Profile: network.Profile{Mode: network.RouteDirect}}}
			err := c.cleanupDeletionKeys(ctx, f.d.AccountID, rt)
			if (err != nil) != wantError {
				t.Errorf("outcome err=%v wantError=%v", err, wantError)
			}
			if fake.deletes != wantDeletes {
				t.Errorf("delete calls=%d want=%d", fake.deletes, wantDeletes)
			}
			var succeeded bool
			if e := db.QueryRow("SELECT EXISTS(SELECT 1 FROM account_deletion_keys WHERE account_id=$1 AND state='SUCCEEDED')", f.d.AccountID).Scan(&succeeded); e != nil {
				t.Fatal(e)
			}
			if succeeded == wantError {
				t.Errorf("unproven deletion was confirmed: succeeded=%v", succeeded)
			}
			if wantError {
				// A new, valid present->absent observation permits retry without losing ownership.
				fake.steps = []deletionKeyRead{{keys: []providers.SSHKey{owned, foreign}}, {keys: []providers.SSHKey{foreign}}}
				fake.reads = 0
				fake.deleteErr = nil
				if e := c.cleanupDeletionKeys(ctx, f.d.AccountID, rt); e != nil {
					t.Fatal("valid retry failed", e)
				}
				var state string
				if e := db.QueryRow("SELECT state FROM account_deletion_keys WHERE account_id=$1 AND provider_key_id='owned-key'", f.d.AccountID).Scan(&state); e != nil || state != "SUCCEEDED" {
					t.Fatal("valid retry not confirmed", state, e)
				}
			}
			var otherState string
			if e := db.QueryRow("SELECT state FROM account_deletion_keys WHERE account_id=$1 AND provider_key_id='other-key'", other.d.AccountID).Scan(&otherState); e != nil || otherState != "RUNNING" {
				t.Fatal("other account changed", otherState, e)
			}
		})
	}
}

func TestDeletionRecoveryValidatesInventoryBeforeAdoption(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c := Container{DB: db}
	for _, scenario := range []string{"empty-owned-id", "invalid-after-owned", "valid-owned"} {
		t.Run(scenario, func(t *testing.T) {
			f := newBootstrapFixture(t, db)
			execBootstrap(t, db, "UPDATE accounts SET enabled=false,deletion_requested_at=now() WHERE id=$1", f.d.AccountID)
			tag := "dob-deployment-" + f.d.ID
			servers := []providers.Server{{ID: "recovered-server", Tags: []string{tag}}}
			if scenario == "empty-owned-id" {
				servers[0].ID = ""
			}
			if scenario == "invalid-after-owned" {
				servers = append(servers, providers.Server{})
			}
			err := c.recoverDeletingInventory(ctx, f.d.AccountID, servers)
			if err == nil {
				t.Fatal("cleanup must remain pending")
			}
			var adopted int
			if e := db.QueryRow("SELECT count(*) FROM droplets WHERE account_id=$1 AND id<>$2", f.d.AccountID, f.d.DropletID).Scan(&adopted); e != nil {
				t.Fatal(e)
			}
			want := 0
			if scenario == "valid-owned" {
				want = 1
			}
			if adopted != want {
				t.Errorf("adopted=%d want=%d; invalid inventory changed deletion ownership", adopted, want)
			}
		})
	}
}
