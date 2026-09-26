package adminapi

import (
	"context"
	"database/sql"
)

func networkIdentityCollision(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, accountID, proxyID string) (bool, error) {
	var collision bool
	err := q.QueryRowContext(ctx, `
WITH candidate AS (
 SELECT p.exit_ip AS exit_ip,
   CASE WHEN p.exit_ip IS NULL THEN NULL WHEN family(p.exit_ip)=4 THEN host(network(set_masklen(p.exit_ip,24)))||'/24' ELSE host(network(set_masklen(p.exit_ip,48)))||'/48' END AS subnet_key
 FROM proxies p
 WHERE p.id=$2
)
SELECT EXISTS(
 SELECT 1 FROM candidate c
 JOIN account_network_identities other ON other.account_id<>$1
 JOIN network_profiles np ON np.account_id=other.account_id AND np.mode='proxy_required'
 WHERE c.exit_ip IS NOT NULL
   AND (other.exit_ip=c.exit_ip OR (c.subnet_key IS NOT NULL AND other.subnet_key=c.subnet_key))
)`, accountID, proxyID).Scan(&collision)
	return collision, err
}
