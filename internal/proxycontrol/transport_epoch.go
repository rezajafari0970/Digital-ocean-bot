package proxycontrol

import (
	"context"
	"database/sql"
	"errors"
)

var ErrTransportEpochUnavailable = errors.New("account transport epoch unavailable")

type TransportEpochStore struct{ DB *sql.DB }

func (s TransportEpochStore) Ensure(ctx context.Context, accountID, provider string, proxyID *string) (int64, error) {
	if s.DB == nil {
		return 0, ErrTransportEpochUnavailable
	}
	var epoch int64
	err := s.DB.QueryRowContext(ctx, `INSERT INTO account_transport_state(account_id,provider,transport_epoch,active_proxy_id,transition_reason)
VALUES($1,$2,1,$3::uuid,'runtime-init')
ON CONFLICT(account_id) DO UPDATE SET provider=EXCLUDED.provider
RETURNING transport_epoch`, accountID, provider, nullableProxyID(proxyID)).Scan(&epoch)
	return epoch, err
}

func (s TransportEpochStore) CurrentEpoch(ctx context.Context, accountID, provider string) (int64, bool, error) {
	if s.DB == nil {
		return 0, false, ErrTransportEpochUnavailable
	}
	var epoch int64
	err := s.DB.QueryRowContext(ctx, `SELECT transport_epoch FROM account_transport_state WHERE account_id=$1 AND provider=$2`, accountID, provider).Scan(&epoch)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return epoch, err == nil, err
}

func (s TransportEpochStore) BumpEpoch(ctx context.Context, accountID, provider string, proxyID *string, reason string) (int64, error) {
	if s.DB == nil {
		return 0, ErrTransportEpochUnavailable
	}
	var epoch int64
	err := s.DB.QueryRowContext(ctx, `INSERT INTO account_transport_state(account_id,provider,transport_epoch,active_proxy_id,transition_reason)
VALUES($1,$2,1,$3::uuid,$4)
ON CONFLICT(account_id) DO UPDATE SET provider=EXCLUDED.provider,transport_epoch=account_transport_state.transport_epoch+1,active_proxy_id=EXCLUDED.active_proxy_id,transition_reason=EXCLUDED.transition_reason,updated_at=now()
RETURNING transport_epoch`, accountID, provider, nullableProxyID(proxyID), reason).Scan(&epoch)
	return epoch, err
}

func nullableProxyID(proxyID *string) any {
	if proxyID == nil || *proxyID == "" {
		return nil
	}
	return *proxyID
}
