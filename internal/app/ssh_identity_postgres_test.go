package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
	"golang.org/x/crypto/ssh"
)

type sshIdentityDriver struct {
	provider string
	inline   bool
	creates  atomic.Int32
	deletes  atomic.Int32
	create   func(context.Context, string, string) (providers.SSHKey, error)
	list     func(context.Context) ([]providers.SSHKey, error)
}

func (d *sshIdentityDriver) Name() string { return d.provider }
func (d *sshIdentityDriver) Capabilities() providers.Capabilities {
	return providers.Capabilities{SSHKeys: true, InlineSSHKeys: d.inline}
}
func (d *sshIdentityDriver) Health(context.Context) error { return nil }
func (d *sshIdentityDriver) CreateSSHKey(ctx context.Context, name, key string) (providers.SSHKey, error) {
	d.creates.Add(1)
	return d.create(ctx, name, key)
}
func (d *sshIdentityDriver) DeleteSSHKey(context.Context, string) error { d.deletes.Add(1); return nil }
func (d *sshIdentityDriver) ListSSHKeys(ctx context.Context) ([]providers.SSHKey, error) {
	if d.list == nil {
		return nil, errors.New("inventory unavailable")
	}
	return d.list(ctx)
}

type sshIdentityFactory struct{ d *sshIdentityDriver }

func (f sshIdentityFactory) Name() string { return f.d.provider }
func (f sshIdentityFactory) Metadata() providers.Metadata {
	return providers.Metadata{Name: f.d.provider}
}
func (f sshIdentityFactory) Open(context.Context, providers.OpenRequest) (providers.Driver, error) {
	return f.d, nil
}

func sshIdentityFixture(t *testing.T, db *sql.DB, driver *sshIdentityDriver) (Container, workflow.Deployment) {
	t.Helper()
	var d workflow.Deployment
	if err := db.QueryRow("INSERT INTO accounts(id,provider,name,secret_ref) VALUES(gen_random_uuid(),$1,'ssh-durability','fixture-credential') RETURNING id::text", driver.provider).Scan(&d.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO network_profiles(id,account_id,mode) VALUES(gen_random_uuid(),$1,'direct')", d.AccountID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("INSERT INTO deployment_profiles(id,account_id,name,config) VALUES(gen_random_uuid(),$1,'ssh-durability','{}') RETURNING id::text", d.AccountID).Scan(&d.ProfileID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("INSERT INTO deployments(id,account_id,profile_id,state,current_step,profile_snapshot) VALUES(gen_random_uuid(),$1,$2,'PLANNED','create','{\"region\":\"test\",\"future_field\":\"preserve\"}') RETURNING id::text", d.AccountID, d.ProfileID).Scan(&d.ID); err != nil {
		t.Fatal(err)
	}
	d.State = workflow.Planned
	d.CurrentStep = "create"
	store, err := secrets.NewStore(secrets.SQLRepository{DB: db}, make([]byte, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Put(context.Background(), d.AccountID, "fixture-credential", "provider", []byte("synthetic-fixture-only")); err != nil {
		t.Fatal(err)
	}
	registry := providers.NewRegistry()
	if err = registry.Register(sshIdentityFactory{driver}); err != nil {
		t.Fatal(err)
	}
	return Container{DB: db, Secrets: store, Accounts: Repository{DB: db}, Providers: registry}, d
}
func sshIdentitySnapshot(t *testing.T, c Container, d workflow.Deployment) workflow.ProfileSnapshot {
	t.Helper()
	s, err := (workflow.ProfileStore{DB: c.DB}).SnapshotForDeployment(context.Background(), d.ID)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func sshIdentityEvidence(t *testing.T, c Container, d workflow.Deployment, key string) []byte {
	t.Helper()
	s := sshIdentitySnapshot(t, c, d)
	plain, err := c.Secrets.Get(context.Background(), d.AccountID, "ssh-deploy-"+d.ID)
	if err != nil {
		t.Fatalf("private key must be durable before provider dispatch: %v", err)
	}
	signer, err := ssh.ParsePrivateKey(plain)
	if err != nil {
		t.Fatal(err)
	}
	if string(ssh.MarshalAuthorizedKey(signer.PublicKey())) != key || s.SSHKeySecretRef != "ssh-deploy-"+d.ID {
		t.Fatal("durable key/snapshot does not match dispatched identity")
	}
	var raw map[string]json.RawMessage
	var b []byte
	if err = c.DB.QueryRow("SELECT profile_snapshot FROM deployments WHERE id=$1", d.ID).Scan(&b); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["future_field"]) != "\"preserve\"" {
		t.Fatal("unrelated snapshot field lost")
	}
	return plain
}

func TestSSHIdentityPersistsBeforeDispatchPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	driver := &sshIdentityDriver{provider: "vultr"}
	c, d := sshIdentityFixture(t, db, driver)
	driver.create = func(ctx context.Context, name, key string) (providers.SSHKey, error) {
		plain := sshIdentityEvidence(t, c, d, key)
		wipe(plain)
		return providers.SSHKey{ID: "key-owned", Name: name}, nil
	}
	s := sshIdentitySnapshot(t, c, d)
	if err := c.ensureDeploymentSSHIdentity(context.Background(), d.AccountID, d, &s); err != nil {
		t.Fatal(err)
	}
	if s.SSHProviderKeyID != "key-owned" || driver.creates.Load() != 1 {
		t.Fatal("identity was not finalized")
	}
}

func TestSSHIdentityUnknownNeverRedispatchesPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	driver := &sshIdentityDriver{provider: "vultr"}
	c, d := sshIdentityFixture(t, db, driver)
	driver.create = func(context.Context, string, string) (providers.SSHKey, error) {
		return providers.SSHKey{}, errors.New("lost provider response")
	}
	for i := 0; i < 3; i++ {
		s := sshIdentitySnapshot(t, c, d)
		if err := c.ensureDeploymentSSHIdentity(context.Background(), d.AccountID, d, &s); err == nil {
			t.Fatal("unknown import accepted")
		}
	}
	if driver.creates.Load() != 1 || driver.deletes.Load() != 0 {
		t.Fatalf("creates=%d deletes=%d; unknown import must never be replayed", driver.creates.Load(), driver.deletes.Load())
	}
}

func sshIdentityPublic(t *testing.T, c Container, d workflow.Deployment) string {
	t.Helper()
	plain, err := c.Secrets.Get(context.Background(), d.AccountID, "ssh-deploy-"+d.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer wipe(plain)
	signer, err := ssh.ParsePrivateKey(plain)
	if err != nil {
		t.Fatal(err)
	}
	return string(ssh.MarshalAuthorizedKey(signer.PublicKey()))
}
func TestSSHIdentityLostResponseReconcilesExactKeyPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	for _, scenario := range []string{"lost-response", "cancelled", "finalize-failed"} {
		t.Run(scenario, func(t *testing.T) {
			driver := &sshIdentityDriver{provider: "vultr"}
			c, d := sshIdentityFixture(t, db, driver)
			callCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			var uploaded providers.SSHKey
			driver.create = func(_ context.Context, name, key string) (providers.SSHKey, error) {
				plain := sshIdentityEvidence(t, c, d, key)
				wipe(plain)
				uploaded = providers.SSHKey{ID: "owned-import", Name: name, PublicKey: key}
				if scenario == "cancelled" {
					cancel()
					return providers.SSHKey{}, context.Canceled
				}
				if scenario == "lost-response" {
					return providers.SSHKey{}, errors.New("lost response")
				}
				return uploaded, nil
			}
			if scenario == "finalize-failed" {
				execBootstrap(t, db, "CREATE FUNCTION fail_ssh_ready() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.profile_snapshot->'ssh_key_intent'->>'state'='ready' THEN RAISE EXCEPTION 'fixture finalization fault'; END IF; RETURN NEW; END $$")
				execBootstrap(t, db, "CREATE TRIGGER fail_ssh_ready BEFORE UPDATE ON deployments FOR EACH ROW EXECUTE FUNCTION fail_ssh_ready()")
			}
			s := sshIdentitySnapshot(t, c, d)
			if err := c.ensureDeploymentSSHIdentity(callCtx, d.AccountID, d, &s); err == nil {
				t.Fatal("failure not surfaced")
			}
			if scenario == "finalize-failed" {
				execBootstrap(t, db, "DROP TRIGGER fail_ssh_ready ON deployments")
				execBootstrap(t, db, "DROP FUNCTION fail_ssh_ready()")
			}
			before := sshIdentityPublic(t, c, d)
			driver.list = func(context.Context) ([]providers.SSHKey, error) { return []providers.SSHKey{uploaded}, nil }
			s = workflow.ProfileSnapshot{} // A restarted process has no in-memory identity.
			if err := c.ensureDeploymentSSHIdentity(ctx, d.AccountID, d, &s); err != nil {
				t.Fatal(err)
			}
			if s.SSHProviderKeyID != "owned-import" || s.SSHKeyIntent.State != "ready" || sshIdentityPublic(t, c, d) != before || driver.creates.Load() != 1 || driver.deletes.Load() != 0 {
				t.Fatal("reconciliation rotated or replayed identity")
			}
		})
	}
}
func TestSSHIdentityRejectsForeignAndIncompleteEvidencePostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	for _, scenario := range []string{"empty", "wrong-name", "wrong-key", "duplicate", "options", "missing-id", "partial-error", "credential-change-during-read"} {
		t.Run(scenario, func(t *testing.T) {
			driver := &sshIdentityDriver{provider: "vultr"}
			c, d := sshIdentityFixture(t, db, driver)
			driver.create = func(context.Context, string, string) (providers.SSHKey, error) {
				return providers.SSHKey{}, errors.New("lost")
			}
			s := sshIdentitySnapshot(t, c, d)
			_ = c.ensureDeploymentSSHIdentity(ctx, d.AccountID, d, &s)
			key := providers.SSHKey{ID: "owned", Name: "dob-" + d.ID, PublicKey: sshIdentityPublic(t, c, d)}
			keys := []providers.SSHKey{key}
			switch scenario {
			case "empty":
				keys = nil
			case "wrong-name":
				keys[0].Name = "foreign"
			case "wrong-key":
				keys[0].PublicKey = "invalid"
			case "duplicate":
				keys = append(keys, key)
			case "options":
				keys[0].PublicKey = "no-pty " + key.PublicKey
			case "missing-id":
				keys[0].ID = ""
			}
			driver.list = func(context.Context) ([]providers.SSHKey, error) {
				if scenario == "partial-error" {
					return keys, errors.New("second page failed")
				}
				if scenario == "credential-change-during-read" {
					if err := c.Secrets.Put(ctx, d.AccountID, "fixture-credential", "provider", []byte("replacement-fixture")); err != nil {
						t.Fatal(err)
					}
				}
				return keys, nil
			}
			if err := c.ensureDeploymentSSHIdentity(ctx, d.AccountID, d, &s); err == nil {
				t.Fatal("unsafe inventory adopted")
			}
			s = sshIdentitySnapshot(t, c, d)
			if s.SSHKeyIntent.State != "unknown" || s.SSHProviderKeyID != "" || driver.creates.Load() != 1 || driver.deletes.Load() != 0 {
				t.Fatal("unproven outcome changed")
			}
		})
	}
}
func TestSSHIdentityConcurrentClaimAndStaleSnapshotPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	driver := &sshIdentityDriver{provider: "vultr"}
	c, d := sshIdentityFixture(t, db, driver)
	entered, release := make(chan struct{}), make(chan struct{})
	driver.create = func(_ context.Context, name, key string) (providers.SSHKey, error) {
		close(entered)
		<-release
		return providers.SSHKey{ID: "only-key", Name: name, PublicKey: key}, nil
	}
	first := make(chan error, 1)
	go func() {
		s := workflow.ProfileSnapshot{}
		first <- c.ensureDeploymentSSHIdentity(ctx, d.AccountID, d, &s)
	}()
	<-entered
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() {
			s := workflow.ProfileSnapshot{}
			results <- c.ensureDeploymentSSHIdentity(ctx, d.AccountID, d, &s)
		}()
	}
	for i := 0; i < 8; i++ {
		if err := <-results; !errors.Is(err, errDeploymentSSHIdentityUnknown) {
			t.Errorf("concurrent result=%v", err)
		}
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	// An old caller snapshot cannot overwrite the completed database identity.
	stale := workflow.ProfileSnapshot{SSHKeySecretRef: "stale", SSHProviderKeyID: "foreign"}
	if err := c.ensureDeploymentSSHIdentity(ctx, d.AccountID, d, &stale); err != nil {
		t.Fatal(err)
	}
	if stale.SSHProviderKeyID != "only-key" || driver.creates.Load() != 1 {
		t.Fatal("stale snapshot replaced committed key")
	}
}
func TestSSHIdentityReservationAtomicityAndCrashWindowsPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	t.Run("atomic-reservation", func(t *testing.T) {
		driver := &sshIdentityDriver{provider: "vultr"}
		c, d := sshIdentityFixture(t, db, driver)
		driver.create = func(context.Context, string, string) (providers.SSHKey, error) {
			t.Fatal("dispatch before reservation committed")
			return providers.SSHKey{}, nil
		}
		execBootstrap(t, db, "CREATE FUNCTION fail_ssh_reserve() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.profile_snapshot ? 'ssh_key_intent' THEN RAISE EXCEPTION 'fixture reservation fault'; END IF; RETURN NEW; END $$")
		execBootstrap(t, db, "CREATE TRIGGER fail_ssh_reserve BEFORE UPDATE ON deployments FOR EACH ROW EXECUTE FUNCTION fail_ssh_reserve()")
		s := workflow.ProfileSnapshot{}
		if err := c.ensureDeploymentSSHIdentity(ctx, d.AccountID, d, &s); err == nil {
			t.Fatal("reservation fault ignored")
		}
		if _, err := c.Secrets.Get(ctx, d.AccountID, "ssh-deploy-"+d.ID); !errors.Is(err, secrets.ErrSecretNotFound) {
			t.Fatal("rolled-back secret remained")
		}
		if sshIdentitySnapshot(t, c, d).SSHKeySecretRef != "" {
			t.Fatal("partial snapshot remained")
		}
		execBootstrap(t, db, "DROP TRIGGER fail_ssh_reserve ON deployments")
		execBootstrap(t, db, "DROP FUNCTION fail_ssh_reserve()")
	})
	for _, dispatched := range []bool{false, true} {
		t.Run(fmt.Sprintf("durable-fence-%v", dispatched), func(t *testing.T) {
			driver := &sshIdentityDriver{provider: "vultr"}
			c, d := sshIdentityFixture(t, db, driver)
			a, err := c.BeginProviderAttempt(ctx, d.AccountID, false)
			if err != nil {
				t.Fatal(err)
			}
			rt, err := c.Runtime(ctx, d.AccountID)
			if err != nil {
				t.Fatal(err)
			}
			_, public, err := c.reserveDeploymentSSHIdentity(ctx, rt, a, d)
			if err != nil {
				t.Fatal(err)
			}
			if dispatched {
				claimed, e := c.claimDeploymentSSHImport(ctx, rt, a, d)
				if e != nil || !claimed {
					t.Fatalf("claim %v %v", claimed, e)
				}
			}
			driver.create = func(_ context.Context, name, key string) (providers.SSHKey, error) {
				if key != public {
					t.Fatal("reserved identity rotated")
				}
				return providers.SSHKey{ID: "key", Name: name}, nil
			}
			driver.list = func(context.Context) ([]providers.SSHKey, error) { return nil, nil }
			s := workflow.ProfileSnapshot{}
			err = c.ensureDeploymentSSHIdentity(ctx, d.AccountID, d, &s)
			if dispatched {
				if !errors.Is(err, errDeploymentSSHIdentityUnknown) || driver.creates.Load() != 0 {
					t.Fatal("uncertain crash fence replayed")
				}
			} else if err != nil || driver.creates.Load() != 1 {
				t.Fatalf("undispatched reservation did not resume: %v", err)
			}
		})
	}
}
func TestSSHIdentityScopeAndLegacyRecoveryPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	t.Run("credential-rotation", func(t *testing.T) {
		driver := &sshIdentityDriver{provider: "vultr"}
		c, d := sshIdentityFixture(t, db, driver)
		a, _ := c.BeginProviderAttempt(ctx, d.AccountID, false)
		rt, err := c.Runtime(ctx, d.AccountID)
		if err != nil {
			t.Fatal(err)
		}
		_, public, err := c.reserveDeploymentSSHIdentity(ctx, rt, a, d)
		if err != nil {
			t.Fatal(err)
		}
		if err = c.Secrets.Put(ctx, d.AccountID, "fixture-credential", "provider", []byte("replacement-fixture")); err != nil {
			t.Fatal(err)
		}
		if claimed, e := c.claimDeploymentSSHImport(ctx, rt, a, d); e == nil || claimed {
			t.Fatal("obsolete credential claimed dispatch")
		}
		s := workflow.ProfileSnapshot{}
		if err = c.ensureDeploymentSSHIdentity(ctx, d.AccountID, d, &s); err == nil {
			t.Fatal("credential scope change accepted")
		}
		if sshIdentityPublic(t, c, d) != public || driver.creates.Load() != 0 {
			t.Fatal("credential rotation overwrote key")
		}
	})
	t.Run("legacy-orphan", func(t *testing.T) {
		driver := &sshIdentityDriver{provider: "vultr"}
		c, d := sshIdentityFixture(t, db, driver)
		plain, err := newDeploymentSSHPrivateKey()
		if err != nil {
			t.Fatal(err)
		}
		defer wipe(plain)
		if err = c.Secrets.Put(ctx, d.AccountID, "ssh-deploy-"+d.ID, "ssh_private_key", plain); err != nil {
			t.Fatal(err)
		}
		public := sshIdentityPublic(t, c, d)
		driver.list = func(context.Context) ([]providers.SSHKey, error) {
			return []providers.SSHKey{{ID: "legacy", Name: "dob-" + d.ID, PublicKey: public}}, nil
		}
		s := workflow.ProfileSnapshot{}
		if err = c.ensureDeploymentSSHIdentity(ctx, d.AccountID, d, &s); err != nil {
			t.Fatal(err)
		}
		if s.SSHProviderKeyID != "legacy" || driver.creates.Load() != 0 || sshIdentityPublic(t, c, d) != public {
			t.Fatal("legacy orphan was replaced")
		}
	})
	t.Run("owner-and-terminal", func(t *testing.T) {
		driver := &sshIdentityDriver{provider: "vultr"}
		c, d := sshIdentityFixture(t, db, driver)
		otherDriver := &sshIdentityDriver{provider: "vultr"}
		_, other := sshIdentityFixture(t, db, otherDriver)
		stale := workflow.ProfileSnapshot{SSHKeySecretRef: "invented", SSHProviderKeyID: "foreign"}
		wrong := d
		wrong.AccountID = other.AccountID
		if err := c.ensureDeploymentSSHIdentity(ctx, other.AccountID, wrong, &stale); err == nil {
			t.Fatal("cross-account stale snapshot accepted")
		}
		execBootstrap(t, db, "UPDATE deployments SET state='FAILED' WHERE id=$1", d.ID)
		if err := c.ensureDeploymentSSHIdentity(ctx, d.AccountID, d, &stale); err == nil {
			t.Fatal("terminal deployment mutated")
		}
		if driver.creates.Load() != 0 {
			t.Fatal("invalid owner/state reached provider")
		}
	})
}
func TestSSHIdentityInlineAndPermissionFallbackPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	for _, inline := range []bool{false, true} {
		t.Run(fmt.Sprintf("inline-%v", inline), func(t *testing.T) {
			driver := &sshIdentityDriver{provider: "digitalocean", inline: inline}
			c, d := sshIdentityFixture(t, db, driver)
			driver.create = func(_ context.Context, name, key string) (providers.SSHKey, error) {
				plain := sshIdentityEvidence(t, c, d, key)
				wipe(plain)
				return providers.SSHKey{}, &providers.Error{Class: providers.ErrorPermissionDenied, StatusCode: 403}
			}
			s := workflow.ProfileSnapshot{}
			if err := c.ensureDeploymentSSHIdentity(ctx, d.AccountID, d, &s); err != nil {
				t.Fatal(err)
			}
			if s.SSHAuthorizedKey == "" || s.SSHProviderKeyID != "" || s.SSHKeyIntent.State != "ready" {
				t.Fatal("inline identity not persisted")
			}
			if err := c.ensureDeploymentSSHIdentity(ctx, d.AccountID, d, &s); err != nil {
				t.Fatal(err)
			}
			want := int32(1)
			if inline {
				want = 0
			}
			if driver.creates.Load() != want || driver.deletes.Load() != 0 {
				t.Fatal("fallback replayed import")
			}
		})
	}
}

type sshIdentityGenerationStore struct{}

func (sshIdentityGenerationStore) CurrentGeneration(context.Context, string, string, string) (int64, bool, error) {
	return 8, true, nil
}
func TestSSHIdentityStaleGenerationAndSecretCollisionPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	driver := &sshIdentityDriver{provider: "vultr"}
	c, d := sshIdentityFixture(t, db, driver)
	a, err := c.BeginProviderAttempt(ctx, d.AccountID, false)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := c.Runtime(ctx, d.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	proxy := "fixture-proxy"
	rt.Config.Network.ProxyID = &proxy
	rt.TransportGeneration = 7
	rt.GenerationStore = sshIdentityGenerationStore{}
	rt.runtimeStatusStore = nil
	s := workflow.ProfileSnapshot{}
	if err = c.ensureDeploymentSSHIdentityWithRuntime(ctx, rt, a, d, &s); !errors.Is(err, ErrRuntimeGenerationObsolete) {
		t.Fatalf("obsolete generation result=%v", err)
	}
	before := sshIdentityPublic(t, c, d)
	if s.SSHKeyIntent.State != "reserved" || driver.creates.Load() != 0 {
		t.Fatal("obsolete generation dispatched")
	}
	// A conflicting secret writer must not replace the reserved identity.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Secrets.CreateInTransaction(ctx, tx, d.AccountID, "ssh-deploy-"+d.ID, "ssh_private_key", []byte("replacement")); err == nil {
		tx.Rollback()
		t.Fatal("reserved private key replaced")
	}
	tx.Rollback()
	if sshIdentityPublic(t, c, d) != before {
		t.Fatal("collision changed encrypted identity")
	}
}
