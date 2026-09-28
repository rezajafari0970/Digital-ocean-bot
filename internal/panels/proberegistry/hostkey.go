package proberegistry

import (
	"context"
	"database/sql"
	"errors"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
)

type HostKeyPins struct{ DB *sql.DB }

func (h HostKeyPins) VerifyOrPin(ctx context.Context, t provisioning.Target, fingerprint string) error {
	if h.DB == nil || t.DropletID == "" || fingerprint == "" {
		return provisioning.ErrHostKeyMismatch
	}
	res, e := h.DB.ExecContext(ctx, `UPDATE reality_probe_nodes SET ssh_host_key_sha256=$2,updated_at=now() WHERE id=$1 AND ssh_host_key_sha256 IS NULL`, t.DropletID, fingerprint)
	if e != nil {
		return e
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	var pinned string
	e = h.DB.QueryRowContext(ctx, `SELECT COALESCE(ssh_host_key_sha256,'') FROM reality_probe_nodes WHERE id=$1`, t.DropletID).Scan(&pinned)
	if errors.Is(e, sql.ErrNoRows) || e != nil {
		return provisioning.ErrHostKeyMismatch
	}
	if pinned != fingerprint {
		return provisioning.ErrHostKeyMismatch
	}
	return nil
}
