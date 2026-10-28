package adminapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

func (s *Server) accounts(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT a.id::text,a.name,COALESCE(a.email,''),COALESCE(a.preferred_region,''),a.provider,a.enabled,a.created_at,COALESCE(n.mode,''),COALESCE(p.name,''),COALESCE(a.preferred_regions,'[]'::jsonb),COALESCE(a.preferred_sizes,'[]'::jsonb),COALESCE(a.preferred_images,'[]'::jsonb),COALESCE(a.preferred_image,''),COALESCE(a.server_lifetime_min_seconds,a.server_lifetime_seconds),COALESCE(a.server_lifetime_max_seconds,a.server_lifetime_seconds),a.auto_interval_seconds,a.auto_batch_size,a.build_spacing_minutes,a.build_spacing_max_minutes,a.auto_max_concurrent,a.server_lifetime_seconds,COALESCE(n.proxy_id::text,''),a.runtime_status,COALESCE(a.runtime_status_detail,''),a.desired_server_count,a.fallback_any_region,COALESCE((SELECT COALESCE((ps.canonical->'Capacity'->>'ComputeLimit')::int,(ps.data->'Limits'->>'DropletLimit')::int) FROM provider_snapshots ps WHERE ps.account_id=a.id ORDER BY ps.created_at DESC LIMIT 1),0),COALESCE((SELECT (ps.canonical->'Capacity'->>'LimitKnown')::boolean FROM provider_snapshots ps WHERE ps.account_id=a.id AND ps.canonical IS NOT NULL ORDER BY ps.created_at DESC LIMIT 1),true),COALESCE((SELECT COALESCE((ps.canonical->'Capacity'->>'ComputeInUse')::int,jsonb_array_length(COALESCE(ps.data->'Droplets','[]'::jsonb))) FROM provider_snapshots ps WHERE ps.account_id=a.id ORDER BY ps.created_at DESC LIMIT 1),0),(SELECT max(ps.created_at) FROM provider_snapshots ps WHERE ps.account_id=a.id),(SELECT max(d.created_at) FROM droplets d WHERE d.account_id=a.id AND d.state<>'DELETED'),(SELECT max(d.updated_at) FROM droplets d WHERE d.account_id=a.id AND d.state='DELETED'),(SELECT min(d.expires_at) FROM droplets d WHERE d.account_id=a.id AND d.state<>'DELETED' AND d.expires_at IS NOT NULL AND d.expires_at>now()),(SELECT old_limit FROM account_capacity_events ce WHERE ce.account_id=a.id ORDER BY ce.detected_at DESC LIMIT 1),(SELECT new_limit FROM account_capacity_events ce WHERE ce.account_id=a.id ORDER BY ce.detected_at DESC LIMIT 1),(SELECT delta FROM account_capacity_events ce WHERE ce.account_id=a.id ORDER BY ce.detected_at DESC LIMIT 1),(SELECT detected_at FROM account_capacity_events ce WHERE ce.account_id=a.id ORDER BY ce.detected_at DESC LIMIT 1),a.provider_state,COALESCE(a.provider_state_detail,''),COALESCE(a.provider_error_state,''),COALESCE(a.provider_error_detail,''),a.provider_state_at,a.provider_checked_at,a.settings_revision,COALESCE((SELECT to_jsonb(ra)-'account_id' FROM account_rule_application ra WHERE ra.account_id=a.id),'{}'::jsonb) FROM accounts a LEFT JOIN network_profiles n ON n.account_id=a.id LEFT JOIN proxies p ON p.id=n.proxy_id WHERE a.deleted_at IS NULL OR a.runtime_status='DELETE_PENDING' ORDER BY a.created_at DESC`)
	if err != nil {
		writeDatabaseReadError(w, err, "accounts_query_failed")
		return
	}
	defer rows.Close()
	type accountRow struct {
		id                    string
		name                  string
		email                 string
		region                string
		provider              string
		mode                  string
		proxy                 string
		image                 string
		proxyID               string
		runtimeStatus         string
		runtimeDetail         string
		providerStateDB       string
		providerStateDetail   string
		providerErrorState    string
		providerErrorDetail   string
		regions               []byte
		sizes                 []byte
		images                []byte
		ruleApplication       []byte
		interval              int
		batch                 int
		spacingMinutes        int
		spacingMaxMinutes     int
		concurrent            int
		lifetime              int
		lifetimeMin           int
		lifetimeMax           int
		desired               int
		dropletLimit          int
		providerDroplets      int
		enabled               bool
		fallbackAnyRegion     bool
		limitKnown            bool
		settingsRevision      int64
		created               time.Time
		capacityCheckedAt     sql.NullTime
		providerStateAt       sql.NullTime
		providerObservationAt sql.NullTime
		lastDropletCreated    sql.NullTime
		lastDropletDeleted    sql.NullTime
		nextDropletDelete     sql.NullTime
		capacityChangedAt     sql.NullTime
		capacityOld           sql.NullInt64
		capacityNew           sql.NullInt64
		capacityDelta         sql.NullInt64
	}
	records := make([]accountRow, 0)
	for rows.Next() {
		var row accountRow
		if err := rows.Scan(&row.id, &row.name, &row.email, &row.region, &row.provider, &row.enabled, &row.created, &row.mode, &row.proxy, &row.regions, &row.sizes, &row.images, &row.image, &row.lifetimeMin, &row.lifetimeMax, &row.interval, &row.batch, &row.spacingMinutes, &row.spacingMaxMinutes, &row.concurrent, &row.lifetime, &row.proxyID, &row.runtimeStatus, &row.runtimeDetail, &row.desired, &row.fallbackAnyRegion, &row.dropletLimit, &row.limitKnown, &row.providerDroplets, &row.capacityCheckedAt, &row.lastDropletCreated, &row.lastDropletDeleted, &row.nextDropletDelete, &row.capacityOld, &row.capacityNew, &row.capacityDelta, &row.capacityChangedAt, &row.providerStateDB, &row.providerStateDetail, &row.providerErrorState, &row.providerErrorDetail, &row.providerStateAt, &row.providerObservationAt, &row.settingsRevision, &row.ruleApplication); err != nil {
			writeDatabaseReadError(w, err, "accounts_scan_failed")
			return
		}
		records = append(records, row)
	}
	if err := rows.Err(); err != nil {
		writeDatabaseReadError(w, err, "accounts_query_failed")
		return
	}
	if err := rows.Close(); err != nil {
		writeDatabaseReadError(w, err, "accounts_query_failed")
		return
	}
	out := make([]map[string]any, 0, len(records))
	for _, row := range records {
		id := row.id
		name := row.name
		email := row.email
		region := row.region
		provider := row.provider
		mode := row.mode
		proxy := row.proxy
		image := row.image
		proxyID := row.proxyID
		runtimeStatus := row.runtimeStatus
		runtimeDetail := row.runtimeDetail
		providerErrorState := row.providerErrorState
		regions := row.regions
		sizes := row.sizes
		images := row.images
		ruleApplication := row.ruleApplication
		interval := row.interval
		batch := row.batch
		spacingMinutes := row.spacingMinutes
		spacingMaxMinutes := row.spacingMaxMinutes
		concurrent := row.concurrent
		lifetime := row.lifetime
		lifetimeMin := row.lifetimeMin
		lifetimeMax := row.lifetimeMax
		desired := row.desired
		dropletLimit := row.dropletLimit
		providerDroplets := row.providerDroplets
		enabled := row.enabled
		fallbackAnyRegion := row.fallbackAnyRegion
		limitKnown := row.limitKnown
		settingsRevision := row.settingsRevision
		created := row.created
		capacityCheckedAt := row.capacityCheckedAt
		providerStateAt := row.providerStateAt
		providerObservationAt := row.providerObservationAt
		lastDropletCreated := row.lastDropletCreated
		lastDropletDeleted := row.lastDropletDeleted
		nextDropletDelete := row.nextDropletDelete
		capacityChangedAt := row.capacityChangedAt
		capacityOld := row.capacityOld
		capacityNew := row.capacityNew
		capacityDelta := row.capacityDelta
		var providerChecked, providerObserved, capacityChecked, ldc, ldd, ndd, cca any
		providerFreshness := "never"
		capacityFreshness := "never"
		if providerStateAt.Valid {
			providerObserved = providerStateAt.Time
		}
		if providerObservationAt.Valid {
			providerChecked = providerObservationAt.Time
			providerFreshness = observationFreshness(providerObservationAt.Time, time.Now())
		}
		if capacityCheckedAt.Valid {
			capacityChecked = capacityCheckedAt.Time
			capacityFreshness = observationFreshness(capacityCheckedAt.Time, time.Now())
		}
		if lastDropletCreated.Valid {
			ldc = lastDropletCreated.Time
		}
		if lastDropletDeleted.Valid {
			ldd = lastDropletDeleted.Time
		}
		if nextDropletDelete.Valid {
			ndd = nextDropletDelete.Time
		}
		if capacityChangedAt.Valid {
			cca = capacityChangedAt.Time
		}
		builtRegionsRaw := []byte("[]")
		if err := s.DB.QueryRowContext(r.Context(), `SELECT COALESCE(jsonb_agg(region ORDER BY region),'[]'::jsonb) FROM (SELECT DISTINCT x->>'RegionID' region FROM (SELECT canonical FROM provider_snapshots WHERE account_id=$1 AND canonical IS NOT NULL ORDER BY created_at DESC LIMIT 1) ps CROSS JOIN LATERAL jsonb_array_elements(COALESCE(NULLIF(ps.canonical->'Inventory'->'Servers','null'::jsonb),'[]'::jsonb)) x WHERE COALESCE(x->>'RegionID','')<>'') q`, id).Scan(&builtRegionsRaw); err != nil {
			writeDatabaseReadError(w, err, "accounts_regions_failed")
			return
		}
		var co, cn, cd any
		if capacityOld.Valid {
			co = capacityOld.Int64
		}
		if capacityNew.Valid {
			cn = capacityNew.Int64
		}
		if capacityDelta.Valid {
			cd = capacityDelta.Int64
		}
		build, buildErr := s.accountBuildState(r.Context(), id)
		if buildErr != nil {
			writeDatabaseReadError(w, buildErr, "accounts_build_state_failed")
			return
		}
		providerState, providerReason := build.State, build.Reason
		providerCanCreate, schedulerCanBuild := build.ProviderCanCreate, build.SchedulerCanBuild
		canCreate, schedulerReason := schedulerCanBuild, build.SchedulerReason
		providerFreshness, capacityFreshness = build.ProviderFreshness, build.CapacityFreshness
		dropletLimit, providerDroplets, limitKnown = build.Limit, build.InUse, build.LimitKnown
		var proxyIDs []string
		pr, proxyErr := s.DB.QueryContext(r.Context(), `SELECT proxy_id::text FROM account_proxy_pool WHERE account_id=$1 AND enabled=true ORDER BY priority,proxy_id`, id)
		if proxyErr != nil {
			writeJSON(w, http.StatusInternalServerError, errorBody())
			return
		}
		if pr != nil {
			for pr.Next() {
				var pid string
				if pr.Scan(&pid) == nil {
					proxyIDs = append(proxyIDs, pid)
				} else {
					pr.Close()
					writeJSON(w, http.StatusInternalServerError, errorBody())
					return
				}
			}
			if err := pr.Err(); err != nil {
				pr.Close()
				writeJSON(w, http.StatusInternalServerError, errorBody())
				return
			}
			pr.Close()
		}
		var managedActive int
		if err := s.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM droplets WHERE account_id=$1 AND state<>'DELETED'`, id).Scan(&managedActive); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorBody())
			return
		}
		desiredRemaining := build.DesiredRemaining
		evidence := s.capacityEvidenceForAccount(id, provider)
		out = append(out, map[string]any{"id": id, "settings_revision": settingsRevision, "name": name, "email": email, "provider": provider, "region": region, "network": mode, "proxy": proxy, "enabled": enabled, "added_at": created.UTC().Format("2006-01-02 15:04:05"), "regions": json.RawMessage(regions), "built_regions": json.RawMessage(builtRegionsRaw), "sizes": json.RawMessage(sizes), "images": json.RawMessage(images), "image": image, "lifetime_min_seconds": lifetimeMin, "lifetime_max_seconds": lifetimeMax, "interval_seconds": interval, "batch_size": batch, "build_spacing_minutes": spacingMinutes, "build_spacing_max_minutes": spacingMaxMinutes, "max_concurrent": concurrent, "lifetime_seconds": lifetime, "proxy_id": proxyID, "proxy_ids": proxyIDs, "billing": s.accountBilling(r.Context(), id), "deletion": s.accountDeletionProgress(r.Context(), id), "rule_application": json.RawMessage(ruleApplication), "runtime_status": runtimeStatus, "runtime_status_detail": runtimeDetail, "desired_server_count": desired, "desired_remaining": desiredRemaining, "capacity_state": evidence.State, "buildable_now": build.Buildable, "pending_builds": build.Pending, "plan_available": build.PlanAvailable, "create_block": build.Block, "upcloud_trial_compatible": build.TrialCompatible, "trial_proxy_candidates": build.TrialProxyCandidates, "trial_relay": build.TrialRelay, "capacity_lower_bound": evidence.LowerBound, "capacity_source": evidence.Source, "capacity_probe_in_flight": evidence.ProbeInFlight, "capacity_probe_after": evidence.ProbeAfter, "fallback_any_region": fallbackAnyRegion, "provider_state": providerState, "can_create": canCreate, "provider_can_create": providerCanCreate, "scheduler_can_build": schedulerCanBuild, "provider_reason": providerReason, "scheduler_reason": schedulerReason, "provider_checked_at": providerChecked, "provider_observed_at": providerObserved, "provider_freshness": providerFreshness, "provider_error_state": providerErrorState, "capacity_checked_at": capacityChecked, "capacity_freshness": capacityFreshness, "droplet_limit": dropletLimit, "capacity_limit_known": limitKnown, "provider_droplets": providerDroplets, "managed_servers": managedActive, "droplet_available": func() any {
			if !limitKnown {
				return nil
			}
			return max(0, dropletLimit-providerDroplets)
		}(), "last_droplet_created": ldc, "last_droplet_deleted": ldd, "next_droplet_delete": ndd, "capacity_old": co, "capacity_new": cn, "capacity_delta": cd, "capacity_changed_at": cca})
	}
	if err := rows.Err(); err != nil {
		writeDatabaseReadError(w, err, "accounts_query_failed")
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) proxies(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT id::text,name,type,host,port,status,COALESCE(host(exit_ip),''),COALESCE(country,''),COALESCE(asn,''),COALESCE(latency_ms,0),failure_count,last_checked_at,COALESCE(adapter,'generic') FROM proxies ORDER BY name`)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, name, typ, host, status, ip, country, asn, adapter string
		var port, fail int
		var latency int64
		var checked any
		if rows.Scan(&id, &name, &typ, &host, &port, &status, &ip, &country, &asn, &latency, &fail, &checked, &adapter) != nil {
			rows.Close()
			writeJSON(w, http.StatusInternalServerError, errorBody())
			return
		}
		out = append(out, map[string]any{"id": id, "name": name, "type": typ, "host": host, "port": port, "status": status, "exit_ip": ip, "country": country, "asn": asn, "latency_ms": latency, "failure_count": fail, "last_checked_at": checked, "adapter": adapter})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeJSON(w, http.StatusInternalServerError, errorBody())
		return
	}
	writeJSON(w, 200, out)
}

func errorBody() map[string]string { return map[string]string{"error": "internal_error"} }
