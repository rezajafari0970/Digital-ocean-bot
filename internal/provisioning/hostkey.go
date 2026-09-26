package provisioning

import (
	"context"
	"database/sql"
	"errors"
)

var ErrHostKeyMismatch = errors.New("ssh host key mismatch")
var ErrHostKeyVerifierMissing = errors.New("ssh host key verifier missing")

type HostKeyPins interface {
	VerifyOrPin(context.Context, Target, string) error
}

type SQLHostKeyPins struct{ DB *sql.DB }

func (s SQLHostKeyPins) VerifyOrPin(ctx context.Context, t Target, fingerprint string) error {
	if s.DB == nil || t.AccountID == "" || t.DropletID == "" || fingerprint == "" {
		return ErrHostKeyMismatch
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE provision_runs SET ssh_host_key_sha256=$3,updated_at=now() WHERE account_id=$1 AND droplet_id=$2 AND ssh_host_key_sha256 IS NULL`, t.AccountID, t.DropletID, fingerprint)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	var pinned string
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(ssh_host_key_sha256,'') FROM provision_runs WHERE account_id=$1 AND droplet_id=$2`, t.AccountID, t.DropletID).Scan(&pinned); err != nil {
		return err
	}
	if pinned != fingerprint {
		return ErrHostKeyMismatch
	}
	return nil
}
