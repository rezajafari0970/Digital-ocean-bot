package migrate

import (
	"context"
	"database/sql"
	"fmt"
)

// Only migration 184 may delegate the legacy enabled flag, and only after an
// exact, complete copy into the new ownership model. All legacy columns, rows,
// secrets and sequences remain covered by the ordinary retention comparison.
func verifyProfileAdoption(ctx context.Context, tx *sql.Tx) error {
	var valid bool
	e := tx.QueryRowContext(ctx, `
 SELECT
 NOT EXISTS(
  SELECT 1 FROM dob_profile_original_policy o
  WHERE ((o.data->>'enabled')::boolean OR EXISTS(SELECT 1 FROM trojan_tcp_listeners))
    AND ((o.data->>'direct_enabled')::boolean OR (o.data->>'residential_enabled')::boolean)
    AND NOT EXISTS(
     SELECT 1 FROM config_driver_profiles p WHERE p.legacy_key='trojan'
     AND p.id='00000000-0000-4000-8000-000000000001'
     AND p.enabled=(o.data->>'enabled')::boolean
     AND p.fleet=(o.data->>'fleet')::boolean
     AND to_jsonb(p.panel_ids)=o.data->'panel_ids'
     AND p.revision=(o.data->>'revision')::bigint
     AND p.clients_per_class=1
     AND p.spec=jsonb_build_object('protocol','trojan','network','tcp','security','tls','port',(o.data->>'port')::integer)
     AND p.classes=array_remove(ARRAY[CASE WHEN (o.data->>'direct_enabled')::boolean THEN 'DIRECT' END,CASE WHEN (o.data->>'residential_enabled')::boolean THEN 'RESIDENTIAL' END],NULL)
    )
 )
 AND NOT EXISTS(
  SELECT 1 FROM trojan_tcp_listeners t JOIN config_driver_profiles p ON p.legacy_key='trojan'
  WHERE NOT EXISTS(
   SELECT 1 FROM config_driver_listeners l WHERE l.profile_id=p.id AND l.panel_id=t.panel_id
   AND l.managed_key=t.managed_key AND l.remote_id IS NOT DISTINCT FROM t.remote_id
   AND l.credential_ref=t.credential_ref AND l.applied_spec=p.spec AND l.applied_revision=t.policy_revision
   AND l.state=CASE WHEN t.state='TLS_READY' THEN 'PENDING' ELSE t.state END
   AND l.stage=t.stage AND l.error_code=t.error_code
   AND l.certificate_not_after IS NOT DISTINCT FROM t.certificate_not_after
   AND l.verified_at IS NOT DISTINCT FROM t.verified_at
  )
 )
 AND NOT EXISTS(
  SELECT 1 FROM trojan_tcp_clients c JOIN config_driver_profiles p ON p.legacy_key='trojan'
  WHERE NOT EXISTS(SELECT 1 FROM config_driver_clients n WHERE n.profile_id=p.id
   AND n.panel_id=c.panel_id AND n.client_id=c.client_id AND n.email=c.email AND n.route_class=c.route_class)
 )
 AND NOT EXISTS(
  SELECT 1 FROM output_config_snapshots o JOIN trojan_tcp_clients c ON c.panel_id=o.panel_id AND c.client_id=o.client_id
  JOIN config_driver_profiles p ON p.legacy_key='trojan'
  WHERE o.uri LIKE 'trojan://%' AND (o.config_profile_id IS DISTINCT FROM p.id OR o.config_profile_revision IS DISTINCT FROM p.revision)
 )
 `).Scan(&valid)
	if e != nil {
		return e
	}
	if !valid {
		return fmt.Errorf("data retention blocked incomplete profile adoption")
	}
	return nil
}
