package app

import "context"

// Final inventory and SSH-key verification still need the account's isolated
// route after its last server is gone. Historical archives are not reactivated.
func (c Container) AccountNetworkMaintenanceIDs(ctx context.Context) ([]string, error) {
	rows, err := c.DB.QueryContext(ctx, `SELECT a.id::text FROM accounts a JOIN network_profiles np ON np.account_id=a.id
 WHERE np.mode='proxy_required' AND (a.enabled OR (a.deletion_requested_at IS NOT NULL AND
 (EXISTS(SELECT 1 FROM account_deletion_jobs j WHERE j.account_id=a.id)
 OR EXISTS(SELECT 1 FROM droplets d WHERE d.account_id=a.id AND d.state<>'DELETED')))) ORDER BY a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
