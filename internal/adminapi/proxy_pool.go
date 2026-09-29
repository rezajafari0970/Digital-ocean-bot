package adminapi

import (
	"context"
	"database/sql"
)

func normalizeProxyPool(primary string, ids []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids)+1)
	if primary != "" {
		out = append(out, primary)
		seen[primary] = true
	}
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
func syncAccountProxyPool(ctx context.Context, tx *sql.Tx, accountID string, ids []string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE account_proxy_pool SET enabled=false,updated_at=now() WHERE account_id=$1`, accountID); err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `INSERT INTO account_proxy_pool(account_id,proxy_id,priority,enabled) VALUES($1,$2::uuid,$3,true) ON CONFLICT(account_id,proxy_id) DO UPDATE SET priority=EXCLUDED.priority,enabled=true,updated_at=now()`, accountID, id, i); err != nil {
			return err
		}
	}
	return nil
}
