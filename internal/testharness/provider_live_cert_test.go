package testharness

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func TestDigitalOceanLiveDriverCertification(t *testing.T) {
	if os.Getenv("DOB_DO_LIVE_CERT") != "1" {
		t.Skip("set DOB_DO_LIVE_CERT=1 to run provider mutation certification")
	}
	accountName := os.Getenv("DOB_DO_CERT_ACCOUNT")
	if accountName == "" {
		accountName = "S1"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	a, err := app.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	var accountID, runtimeStatus, providerState string
	if err = a.DB.QueryRowContext(ctx,
		"SELECT id::text,runtime_status,provider_state FROM accounts WHERE name=$1 AND deleted_at IS NULL",
		accountName).Scan(&accountID, &runtimeStatus, &providerState); err != nil {
		t.Fatal(err)
	}
	if runtimeStatus != "READY" || providerState != "ACTIVE" {
		t.Fatalf("unsafe certification state runtime=%s provider=%s", runtimeStatus, providerState)
	}

	rt, err := a.Container.Runtime(ctx, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if rt.Driver.Name() != "digitalocean" {
		t.Fatalf("provider=%s", rt.Driver.Name())
	}

	ar := rt.Driver.(providers.AccountReader)
	cr := rt.Driver.(providers.CatalogReader)
	cd := rt.Driver.(providers.ComputeDriver)
	kd := rt.Driver.(providers.SSHKeyDriver)

	cap, err := ar.Capacity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	existing, err := cd.ListServers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cap.ComputeLimit-cap.ComputeInUse < 1 || len(existing) != 0 {
		t.Fatalf("certification account must have free capacity and zero servers: limit=%d inuse=%d list=%d", cap.ComputeLimit, cap.ComputeInUse, len(existing))
	}

	cat, err := cr.Catalog(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var imageID string
	for _, im := range cat.Images {
		if im.Available && im.Family == "ubuntu" && im.Version == "24.04" {
			imageID = im.ID
			break
		}
	}
	if imageID == "" {
		t.Fatal("Ubuntu 24.04 is unavailable")
	}

	var regionID, planID string
	for _, p := range cat.Plans {
		if !p.Available {
			continue
		}
		for _, rid := range p.AvailableRegions {
			for _, r := range cat.Regions {
				if r.ID == rid && r.Available {
					regionID, planID = r.ID, p.ID
					break
				}
			}
			if regionID != "" {
				break
			}
		}
		if regionID != "" {
			break
		}
	}
	if regionID == "" || planID == "" {
		t.Fatal("no compatible region/plan")
	}

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}

	key, err := kd.CreateSSHKey(ctx, "dob-provider-cert", string(ssh.MarshalAuthorizedKey(sshPub)))
	if err != nil {
		t.Fatal(err)
	}
	keyLive := true
	t.Cleanup(func() {
		if keyLive {
			_ = kd.DeleteSSHKey(context.Background(), key.ID)
		}
	})

	identity := fmt.Sprintf("dob-cert-%d", time.Now().UnixNano())
	result, err := cd.CreateServer(ctx, providers.CreateServerRequest{
		Name: "dob-provider-cert", RegionID: regionID, PlanID: planID, ImageID: imageID,
		SSHKeyRefs: []string{key.ID}, Tags: []string{"managed-by-digital-ocean-bot", "provider-certification"}, Identity: identity,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ServerID == "" || result.Outcome != providers.OutcomeAccepted {
		t.Fatalf("create result=%+v", result)
	}
	serverID := result.ServerID
	serverLive := true
	t.Cleanup(func() {
		if serverLive {
			_ = cd.DeleteServer(context.Background(), serverID)
		}
	})

	var ready providers.Server
	deadline := time.Now().Add(150 * time.Second)
	for time.Now().Before(deadline) {
		ready, err = cd.GetServer(ctx, serverID)
		if err == nil && ready.Ready && ready.PrimaryIPv4 != "" {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if !ready.Ready || ready.PrimaryIPv4 == "" {
		t.Fatal("server did not become ready with public IPv4")
	}

	found, err := cd.FindServerByIdentity(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != serverID {
		t.Fatalf("identity recovery mismatch found=%+v server=%s", found, serverID)
	}

	if err = cd.DeleteServer(ctx, serverID); err != nil {
		t.Fatal(err)
	}
	serverLive = false

	gone := false
	for i := 0; i < 45; i++ {
		_, e := cd.GetServer(ctx, serverID)
		if providers.IsClass(e, providers.ErrorNotFound) {
			gone = true
			break
		}
		time.Sleep(2 * time.Second)
	}
	if !gone {
		t.Fatal("server deletion was not confirmed")
	}

	if err = kd.DeleteSSHKey(ctx, key.ID); err != nil {
		t.Fatal(err)
	}
	keyLive = false

	t.Logf("CERT_PASS provider=%s account=%s region=%s plan=%s image=%s server=%s", rt.Driver.Name(), accountName, regionID, planID, imageID, serverID)
}
