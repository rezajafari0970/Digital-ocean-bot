package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
