package app

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
	"golang.org/x/crypto/ssh"
)

func (c Container) ensureDeploymentSSHIdentity(ctx context.Context, accountID string, d workflow.Deployment, snap *workflow.ProfileSnapshot) error {
	if snap.SSHKeySecretRef != "" && snap.SSHProviderKeyID > 0 {
		return nil
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return err
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	defer wipe(privatePEM)
	pub, err := ssh.NewPublicKey(priv.Public())
	if err != nil {
		return err
	}
	runtime, err := c.Runtime(ctx, accountID)
	if err != nil {
		return err
	}
	created, err := runtime.Provider.CreateSSHKey(ctx, fmt.Sprintf("dob-%s", d.ID), string(ssh.MarshalAuthorizedKey(pub)))
	if err != nil {
		return err
	}
	ref := "ssh-deploy-" + d.ID
	if err = c.Secrets.Put(ctx, accountID, ref, "ssh_private_key", privatePEM); err != nil {
		_ = runtime.Provider.DeleteSSHKey(ctx, created.ID)
		return err
	}
	snap.SSHKeySecretRef = ref
	snap.SSHProviderKeyID = created.ID
	snap.SSHUser = "root"
	raw, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	_, err = c.DB.ExecContext(ctx, `UPDATE deployments SET profile_snapshot=$2 WHERE id=$1`, d.ID, raw)
	return err
}
