package configplane

import "context"

// A fresh installation has no legacy server ownership to retire. Activate its
// sole controller automatically; existing installations use explicit reset.
func (s Service) initializeEmpty(ctx context.Context) error {
	var empty bool
	if e := s.DB.QueryRowContext(ctx, "SELECT NOT unified AND NOT EXISTS(SELECT 1 FROM panel_instances) FROM config_control_authority WHERE singleton").Scan(&empty); e != nil {
		return e
	}
	if !empty {
		return nil
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, lock := range []int64{628341902731, 628341902732, 180061015} {
		if _, e = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", lock); e != nil {
			return e
		}
	}
	if e = tx.QueryRowContext(ctx, "SELECT NOT unified AND NOT EXISTS(SELECT 1 FROM panel_instances) FROM config_control_authority WHERE singleton FOR UPDATE").Scan(&empty); e != nil {
		return e
	}
	if !empty {
		return tx.Commit()
	}
	for _, q := range []string{
		"UPDATE global_config_policies SET enabled=false WHERE policy_key='reality'",
		"UPDATE reality_config_profiles SET enabled=false",
		"UPDATE trojan_tcp_policy SET enabled=false",
		"UPDATE bulk_lifecycle_control SET enabled=false,auto_enroll=false",
		"UPDATE bulk_lifecycle_scopes SET enabled=false",
		"UPDATE bulk_client_execution_gate SET enabled=false,kill_switch=true",
		"UPDATE config_control_authority SET unified=true,activated_at=now() WHERE singleton",
		"UPDATE client_mutation_execution_gate SET enabled=true,kill_switch=false,concurrency=1,panel_id=NULL,inbound_id=NULL WHERE singleton",
	} {
		if _, e = tx.ExecContext(ctx, q); e != nil {
			return e
		}
	}
	return tx.Commit()
}
