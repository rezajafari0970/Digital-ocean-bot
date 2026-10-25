package secrets

import (
	"context"
	"errors"
)

// GetIPCleanup decrypts only the credential archived for one dispatched IP operation.
// The original ID is part of AEAD associated data and must never be replaced by the
// operation ID. Callers still need separate durable resource-scoped cleanup authority.
func (s *Store) GetIPCleanup(ctx context.Context, account, operation string) ([]byte, error) {
	repo, ok := s.repo.(SQLRepository)
	if !ok || repo.DB == nil {
		return nil, errors.New("IP cleanup credential repository unavailable")
	}
	var id, kind string
	var ct, nonce []byte
	var version int
	err := repo.DB.QueryRowContext(ctx, `SELECT c.original_id,c.kind,c.ciphertext,c.nonce,c.key_version
 FROM ip_cleanup_credentials c JOIN ip_mutation_intents m ON m.id=c.operation_id
 WHERE c.account_id=$1 AND c.operation_id=$2 AND m.account_id=c.account_id AND m.dispatch_id IS NOT NULL`, account, operation).Scan(&id, &kind, &ct, &nonce, &version)
	if err != nil {
		return nil, err
	}
	_, available := s.keys[version]
	if !available {
		return nil, errors.New("IP cleanup credential key version unavailable")
	}
	aad := []byte(account + "\x00" + id + "\x00" + kind)
	pt, err := s.openRecord(version, nonce, ct, aad)
	if err != nil {
		return nil, errors.New("IP cleanup credential authentication failed")
	}
	return pt, nil
}

// ValidateIPCleanupKeys is also the key-retirement gate. A candidate keyring may
// not start without authenticating every retained archive. Ordinary archive
// updates remain forbidden, so deletion of current credentials cannot lose AAD.
func (s *Store) ValidateIPCleanupKeys(ctx context.Context) error {
	repo, ok := s.repo.(SQLRepository)
	if !ok {
		return nil
	}
	var exists bool
	if e := repo.DB.QueryRowContext(ctx, "SELECT to_regclass('ip_cleanup_credentials') IS NOT NULL").Scan(&exists); e != nil {
		return e
	}
	if !exists {
		return nil
	}
	rows, e := repo.DB.QueryContext(ctx, "SELECT account_id::text,original_id,kind,ciphertext,nonce,key_version FROM ip_cleanup_credentials")
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var account, id, kind string
		var ct, nonce []byte
		var version int
		if e = rows.Scan(&account, &id, &kind, &ct, &nonce, &version); e != nil {
			return e
		}
		_, ok := s.keys[version]
		if !ok {
			return errors.New("retained IP cleanup key cannot be retired")
		}
		pt, e := s.openRecord(version, nonce, ct, []byte(account+"\x00"+id+"\x00"+kind))
		if e != nil {
			return errors.New("retained IP cleanup key authentication failed")
		}
		for i := range pt {
			pt[i] = 0
		}
	}
	return rows.Err()
}
