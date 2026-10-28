package adminapi

import (
	"context"
	"encoding/json"
)

// Seed updates from persisted configuration so an older client omitting a newer
// field cannot reset it. The revision is checked again under the write lock.
func (s *Server) accountSettingsSnapshot(ctx context.Context, id string) (accountUpdate, error) {
	var raw []byte
	err := s.DB.QueryRowContext(ctx, `SELECT jsonb_build_object(
 'name',a.name,'settings_revision',a.settings_revision,
 'regions',a.preferred_regions,'sizes',a.preferred_sizes,
 'images',a.preferred_images,'image',a.preferred_image,
 'lifetime_min_seconds',COALESCE(a.server_lifetime_min_seconds,a.server_lifetime_seconds),
 'lifetime_max_seconds',COALESCE(a.server_lifetime_max_seconds,a.server_lifetime_seconds),
 'build_spacing_minutes',a.build_spacing_minutes,'build_spacing_max_minutes',a.build_spacing_max_minutes,
 'interval_seconds',a.auto_interval_seconds,'batch_size',a.auto_batch_size,'max_concurrent',a.auto_max_concurrent,
 'desired_server_count',a.desired_server_count,'fallback_any_region',a.fallback_any_region,
 'apply_to_existing',COALESCE(ra.apply_to_existing,false),
 'network_mode',COALESCE(np.mode,'direct'),
 'proxy_id',COALESCE((SELECT proxy_id::text FROM account_proxy_pool WHERE account_id=a.id AND enabled ORDER BY priority,proxy_id LIMIT 1),np.proxy_id::text,''),
 'proxy_ids',COALESCE((SELECT jsonb_agg(proxy_id::text ORDER BY priority,proxy_id) FROM account_proxy_pool WHERE account_id=a.id AND enabled),'[]'::jsonb))
 FROM accounts a LEFT JOIN network_profiles np ON np.account_id=a.id
 LEFT JOIN account_rule_application ra ON ra.account_id=a.id WHERE a.id=$1`, id).Scan(&raw)
	var out accountUpdate
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	return out, err
}
