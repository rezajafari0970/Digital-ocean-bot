package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"errors"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
	"golang.org/x/crypto/ssh"
)

func deploymentSSHReady(s workflow.ProfileSnapshot) bool {
	return s.SSHKeySecretRef != "" && (s.SSHProviderKeyID != "" || s.SSHAuthorizedKey != "")
}
func (c Container) deploymentSSHSnapshot(ctx context.Context, accountID, id string) (workflow.ProfileSnapshot, error) {
	var raw []byte
	err := c.DB.QueryRowContext(ctx, "SELECT profile_snapshot FROM deployments WHERE id=$1 AND account_id=$2", id, accountID).Scan(&raw)
	var s workflow.ProfileSnapshot
	if err == nil {
		err = json.Unmarshal(raw, &s)
	}
	return s, err
}

// Lock order is account, credential, deployment. No lock is held over network IO.
func (c Container) lockDeploymentSSHIdentity(ctx context.Context, tx *sql.Tx, rt AccountRuntime, a ProviderAttempt, d workflow.Deployment) (workflow.ProfileSnapshot, string, error) {
	var s workflow.ProfileSnapshot
	valid, err := lockProviderAttempt(ctx, tx, a)
	if err != nil {
		return s, "", err
	}
	if !valid || a.AccountID != d.AccountID || rt.Config.ID != d.AccountID {
		return s, "", errDeploymentSSHIdentityConflict
	}
	var provider, external string
	if err = tx.QueryRowContext(ctx, "SELECT provider,COALESCE(external_id,'') FROM accounts WHERE id=$1", d.AccountID).Scan(&provider, &external); err != nil {
		return s, "", err
	}
	if provider != rt.Config.Provider {
		return s, "", errDeploymentSSHIdentityConflict
	}
	var raw []byte
	var state, step, resource string
	if err = tx.QueryRowContext(ctx, "SELECT profile_snapshot,state,current_step,COALESCE(provider_id,'') FROM deployments WHERE id=$1 AND account_id=$2 FOR UPDATE", d.ID, d.AccountID).Scan(&raw, &state, &step, &resource); err != nil {
		return s, "", err
	}
	if len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' {
		return s, "", errDeploymentSSHIdentityConflict
	}
	if err = json.Unmarshal(raw, &s); err != nil {
		return s, "", err
	}
	if step != "create" || resource != "" || terminalDeploymentStateForCreateRecovery(state) {
		return s, "", errDeploymentSSHIdentityConflict
	}
	if i := s.SSHKeyIntent; i != nil {
		if i.Version != 1 || i.AccountID != d.AccountID || i.Provider != provider || i.ExternalID != external || i.CredentialRef != a.SecretRef ||
			i.CredentialAt.Valid != a.CredentialAt.Valid || (i.CredentialAt.Valid && !i.CredentialAt.Time.Equal(a.CredentialAt.Time)) ||
			s.SSHKeySecretRef != "ssh-deploy-"+d.ID || s.SSHKeyFingerprint == "" ||
			(i.State != "reserved" && i.State != "unknown" && i.State != "ready") {
			return s, "", errDeploymentSSHIdentityConflict
		}
	}
	return s, external, nil
}
func (c Container) checkDeploymentSSHScope(ctx context.Context, rt AccountRuntime, a ProviderAttempt, d workflow.Deployment) error {
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, _, err = c.lockDeploymentSSHIdentity(ctx, tx, rt, a, d)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Merge only identity-owned fields. Preserve forward-compatible and unrelated
// profile fields instead of reserializing a stale typed snapshot.
func writeDeploymentSSHIdentity(ctx context.Context, tx *sql.Tx, d workflow.Deployment, s workflow.ProfileSnapshot) error {
	patch, err := json.Marshal(map[string]any{
		"ssh_key_intent": s.SSHKeyIntent, "ssh_key_secret_ref": s.SSHKeySecretRef,
		"ssh_key_fingerprint": s.SSHKeyFingerprint, "ssh_provider_key_id": s.SSHProviderKeyID,
		"ssh_authorized_key": s.SSHAuthorizedKey, "ssh_user": s.SSHUser,
	})
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, "UPDATE deployments SET lock_version=lock_version+1,profile_snapshot=profile_snapshot||$3::jsonb,updated_at=now() WHERE id=$1 AND account_id=$2", d.ID, d.AccountID, patch)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errDeploymentSSHIdentityConflict
	}
	return nil
}

func (c Container) reserveDeploymentSSHIdentity(ctx context.Context, rt AccountRuntime, a ProviderAttempt, d workflow.Deployment) (workflow.ProfileSnapshot, string, error) {
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return workflow.ProfileSnapshot{}, "", err
	}
	defer tx.Rollback()
	s, external, err := c.lockDeploymentSSHIdentity(ctx, tx, rt, a, d)
	if err != nil {
		return s, "", err
	}
	if s.SSHKeyIntent == nil && deploymentSSHReady(s) {
		return s, "", errDeploymentSSHIdentityConflict
	}
	if _, ok := rt.Driver.(providers.SSHKeyDriver); !ok && !rt.Driver.Capabilities().InlineSSHKeys {
		return s, "", ErrProviderComputeUnsupported
	}
	ref := "ssh-deploy-" + d.ID
	if s.SSHKeySecretRef != "" && s.SSHKeySecretRef != ref {
		return s, "", errDeploymentSSHIdentityConflict
	}
	plain, err := c.Secrets.Get(ctx, d.AccountID, ref)
	fresh := errors.Is(err, secrets.ErrSecretNotFound)
	if err != nil && !fresh {
		return s, "", err
	}
	if fresh {
		if s.SSHKeyIntent != nil || s.SSHKeySecretRef != "" {
			return s, "", errDeploymentSSHIdentityConflict
		}
		plain, err = newDeploymentSSHPrivateKey()
		if err != nil {
			return s, "", err
		}
	}
	defer wipe(plain)
	signer, err := ssh.ParsePrivateKey(plain)
	if err != nil {
		return s, "", errors.New("durable deployment SSH private key is invalid")
	}
	publicKey := string(ssh.MarshalAuthorizedKey(signer.PublicKey()))
	fingerprint := ssh.FingerprintSHA256(signer.PublicKey())
	if s.SSHKeyIntent != nil {
		if s.SSHKeyFingerprint != fingerprint {
			return s, "", errDeploymentSSHIdentityConflict
		}
		if s.SSHKeyIntent.State == "ready" {
			if !deploymentSSHReady(s) || (s.SSHAuthorizedKey != "" && !sameDeploymentSSHPublicKey(s.SSHAuthorizedKey, publicKey)) {
				return s, "", errDeploymentSSHIdentityConflict
			}
		} else if s.SSHProviderKeyID != "" || s.SSHAuthorizedKey != "" {
			return s, "", errDeploymentSSHIdentityConflict
		}
		return s, publicKey, tx.Commit()
	}
	if fresh {
		if err = c.Secrets.CreateInTransaction(ctx, tx, d.AccountID, ref, "ssh_private_key", plain); err != nil {
			return s, "", err
		}
	}
	state := "reserved"
	if !fresh {
		state = "unknown"
	} // A legacy orphan may already have been imported.
	s.SSHKeySecretRef = ref
	s.SSHKeyFingerprint = fingerprint
	s.SSHUser = "root"
	s.SSHProviderKeyID = ""
	s.SSHAuthorizedKey = ""
	s.SSHKeyIntent = &workflow.SSHKeyIntent{Version: 1, AccountID: d.AccountID, Provider: rt.Config.Provider, ExternalID: external, CredentialRef: a.SecretRef, CredentialAt: a.CredentialAt, State: state}
	if err = writeDeploymentSSHIdentity(ctx, tx, d, s); err != nil {
		return s, "", err
	}
	return s, publicKey, tx.Commit()
}

func newDeploymentSSHPrivateKey() ([]byte, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	defer wipe(priv)
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}
	defer wipe(der)
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

func (c Container) claimDeploymentSSHImport(ctx context.Context, rt AccountRuntime, a ProviderAttempt, d workflow.Deployment) (bool, error) {
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	s, _, err := c.lockDeploymentSSHIdentity(ctx, tx, rt, a, d)
	if err != nil {
		return false, err
	}
	if s.SSHKeyIntent == nil || s.SSHKeyIntent.State != "reserved" {
		return false, nil
	}
	s.SSHKeyIntent.State = "unknown"
	if err = writeDeploymentSSHIdentity(ctx, tx, d, s); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (c Container) finishDeploymentSSHIdentity(ctx context.Context, rt AccountRuntime, a ProviderAttempt, d workflow.Deployment, publicKey, id string, inline bool) (workflow.ProfileSnapshot, error) {
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return workflow.ProfileSnapshot{}, err
	}
	defer tx.Rollback()
	s, _, err := c.lockDeploymentSSHIdentity(ctx, tx, rt, a, d)
	if err != nil {
		return s, err
	}
	if s.SSHKeyIntent == nil {
		return s, errDeploymentSSHIdentityConflict
	}
	if s.SSHKeyIntent.State == "ready" {
		if (inline && s.SSHAuthorizedKey == publicKey && s.SSHProviderKeyID == "") || (!inline && s.SSHProviderKeyID.String() == id && s.SSHAuthorizedKey == "") {
			return s, tx.Commit()
		}
		return s, errDeploymentSSHIdentityConflict
	}
	if !inline && (s.SSHKeyIntent.State != "unknown" || id == "") {
		return s, errDeploymentSSHIdentityConflict
	}
	// Verify that the exact encrypted key still exists before publishing READY.
	plain, err := c.Secrets.Get(ctx, d.AccountID, s.SSHKeySecretRef)
	if err != nil {
		return s, err
	}
	defer wipe(plain)
	signer, err := ssh.ParsePrivateKey(plain)
	if err != nil || ssh.FingerprintSHA256(signer.PublicKey()) != s.SSHKeyFingerprint || !sameDeploymentSSHPublicKey(string(ssh.MarshalAuthorizedKey(signer.PublicKey())), publicKey) {
		return s, errDeploymentSSHIdentityConflict
	}
	s.SSHKeyIntent.State = "ready"
	if inline {
		s.SSHAuthorizedKey = publicKey
		s.SSHProviderKeyID = ""
	} else {
		s.SSHProviderKeyID = workflow.ProviderKeyRef(id)
		s.SSHAuthorizedKey = ""
	}
	if err = writeDeploymentSSHIdentity(ctx, tx, d, s); err != nil {
		return s, err
	}
	return s, tx.Commit()
}

func sameDeploymentSSHPublicKey(a, b string) bool {
	ka, _, oa, ra, ea := ssh.ParseAuthorizedKey([]byte(a))
	kb, _, ob, rb, eb := ssh.ParseAuthorizedKey([]byte(b))
	return ea == nil && eb == nil && len(oa) == 0 && len(ob) == 0 && len(bytes.TrimSpace(ra)) == 0 && len(bytes.TrimSpace(rb)) == 0 && bytes.Equal(ka.Marshal(), kb.Marshal())
}
