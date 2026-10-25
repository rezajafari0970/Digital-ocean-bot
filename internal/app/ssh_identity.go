package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
)

var errDeploymentSSHIdentityUnknown = errors.New("deployment SSH key import is unresolved; owned-key reconciliation required")
var errDeploymentSSHIdentityConflict = errors.New("deployment SSH identity scope or durable state changed")

func (c Container) ensureDeploymentSSHIdentity(ctx context.Context, accountID string, d workflow.Deployment, snap *workflow.ProfileSnapshot) error {
	if c.DB == nil || c.Secrets == nil || snap == nil || accountID == "" || d.ID == "" || d.AccountID != accountID {
		return errDeploymentSSHIdentityConflict
	}
	// Caller snapshots can be stale. Never authorize an early return from them.
	current, err := c.deploymentSSHSnapshot(ctx, accountID, d.ID)
	if err != nil {
		return err
	}
	if current.SSHKeyIntent == nil && deploymentSSHReady(current) {
		*snap = current
		return nil // Preserve already provisioned legacy identities.
	}
	attempt, err := c.BeginProviderAttempt(ctx, accountID, false)
	if err != nil {
		return err
	}
	runtime, err := c.Runtime(ctx, accountID)
	if err != nil {
		return err
	}
	return c.ensureDeploymentSSHIdentityWithRuntime(ctx, runtime, attempt, d, snap)
}

func (c Container) ensureDeploymentSSHIdentityWithRuntime(ctx context.Context, runtime AccountRuntime, attempt ProviderAttempt, d workflow.Deployment, snap *workflow.ProfileSnapshot) error {
	current, publicKey, err := c.reserveDeploymentSSHIdentity(ctx, runtime, attempt, d)
	if err != nil {
		return err
	}
	*snap = current
	if current.SSHKeyIntent.State == "ready" {
		return nil
	}
	if err = runtime.CheckMutationGeneration(ctx); err != nil {
		return err
	}
	if runtime.Driver.Capabilities().InlineSSHKeys {
		current, err = c.finishDeploymentSSHIdentity(ctx, runtime, attempt, d, publicKey, "", true)
		if err == nil {
			*snap = current
		}
		return err
	}
	driver, ok := runtime.Driver.(providers.SSHKeyDriver)
	if !ok {
		return ErrProviderComputeUnsupported
	}
	if current.SSHKeyIntent.State == "unknown" {
		return c.reconcileDeploymentSSHIdentity(ctx, runtime, attempt, d, publicKey, snap)
	}
	claimed, err := c.claimDeploymentSSHImport(ctx, runtime, attempt, d)
	if err != nil {
		return err
	}
	if !claimed {
		return errDeploymentSSHIdentityUnknown
	}
	// The durable UNKNOWN fence survives a crash or a lost database session.
	// Only its single successful claimant may send a create. An empty subsequent
	// inventory never proves that a previously dispatched request was rejected.
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = runtime.CheckMutationGeneration(ctx); err != nil {
		return err
	}
	if err = c.checkDeploymentSSHScope(ctx, runtime, attempt, d); err != nil {
		return err
	}
	created, createErr := driver.CreateSSHKey(ctx, "dob-"+d.ID, publicKey)
	fallback := runtime.Config.Provider == "digitalocean" && providers.IsClass(createErr, providers.ErrorPermissionDenied)
	if createErr != nil && !fallback {
		// Do not expose arbitrary provider text or classify an uncertain import as
		// a terminal deployment failure. The next attempt performs only inventory.
		return errDeploymentSSHIdentityUnknown
	}
	if !fallback && strings.TrimSpace(created.ID) == "" {
		return errDeploymentSSHIdentityUnknown
	}
	current, err = c.finishDeploymentSSHIdentity(ctx, runtime, attempt, d, publicKey, created.ID, fallback)
	if err == nil {
		*snap = current
	}
	return err
}

func (c Container) reconcileDeploymentSSHIdentity(ctx context.Context, runtime AccountRuntime, attempt ProviderAttempt, d workflow.Deployment, publicKey string, snap *workflow.ProfileSnapshot) error {
	reader, ok := runtime.Driver.(sshKeyInventory)
	if !ok {
		return errDeploymentSSHIdentityUnknown
	}
	if err := c.checkDeploymentSSHScope(ctx, runtime, attempt, d); err != nil {
		return err
	}
	keys, err := reader.ListSSHKeys(ctx)
	if err != nil {
		return fmt.Errorf("%w: inventory unavailable", errDeploymentSSHIdentityUnknown)
	}
	found := ""
	for _, key := range keys {
		if key.Name != "dob-"+d.ID || !sameDeploymentSSHPublicKey(key.PublicKey, publicKey) {
			continue
		}
		if strings.TrimSpace(key.ID) == "" || found != "" {
			return errDeploymentSSHIdentityUnknown
		}
		found = key.ID
	}
	if found == "" {
		return errDeploymentSSHIdentityUnknown
	}
	if err = runtime.CheckMutationGeneration(ctx); err != nil {
		return err
	}
	current, err := c.finishDeploymentSSHIdentity(ctx, runtime, attempt, d, publicKey, found, false)
	if err == nil {
		*snap = current
	}
	return err
}
