package capacity

import (
	"context"
	"database/sql"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"regexp"
	"time"
)

var ErrCreateBlocked = errors.New("provider create permission blocked; resolve restriction before retry")
var createCode = regexp.MustCompile("^[A-Z0-9_]{1,80}$")

type CreateBlock struct {
	Version   int64     `json:"version"`
	Code      string    `json:"code"`
	BlockedAt time.Time `json:"blocked_at"`
}

func CreateBlockReason(code string) string {
	if code == "TRIAL_FIREWALL" {
		return "UpCloud trial firewall restriction (TRIAL_FIREWALL). This deployment requires ports unavailable during the trial. Resolve the trial restriction with UpCloud, then enable a retry."
	}
	return "UpCloud rejected server creation (" + code + "). Resolve the provider permission restriction, then enable a retry."
}
func ReadCreateBlock(ctx context.Context, q Querier, accountID string) (*CreateBlock, error) {
	var b CreateBlock
	err := q.QueryRowContext(ctx, "SELECT version,code,blocked_at FROM account_create_blocks WHERE account_id=$1", accountID).Scan(&b.Version, &b.Code, &b.BlockedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// Only a definitive UpCloud create denial can establish this independent block.
// Read-only observations never write to this table.
func RecordCreateBlock(ctx context.Context, db *sql.DB, accountID string, createErr error) error {
	var pe *providers.Error
	if !errors.As(createErr, &pe) || pe.Operation != "create_server" || pe.StatusCode != 403 || pe.Class != providers.ErrorPermissionDenied || !createCode.MatchString(pe.Code) {
		return nil
	}
	var version int64
	err := db.QueryRowContext(ctx, `INSERT INTO account_create_blocks(account_id,code)
 SELECT id,$2 FROM accounts WHERE id=$1 AND provider='upcloud'
 ON CONFLICT(account_id) DO UPDATE SET code=EXCLUDED.code,blocked_at=now(),version=nextval('account_create_blocks_version_seq')
 RETURNING version`, accountID, pe.Code).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	// Persist denial even if audit storage fails; never undo a safety block.
	_, err = db.ExecContext(ctx, `INSERT INTO audit_events(account_id,actor,action,resource_type,resource_id,result,message,metadata) VALUES($1,'worker','create_permission_blocked','account',$1::uuid::text,'blocked',$2,jsonb_build_object('block_version',$3::bigint))`, accountID, pe.Code, version)
	if err != nil {
		return err
	}
	return nil
}
