package adminapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

func (s *Server) accounts(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT a.id::text,a.name,COALESCE(a.email,''),COALESCE(a.preferred_region,''),a.provider,a.enabled,a.created_at,n.mode,COALESCE(p.name,''),COALESCE(a.preferred_regions,'[]'::jsonb),COALESCE(a.preferred_sizes,'[]'::jsonb),COALESCE(a.preferred_images,'[]'::jsonb),COALESCE(a.preferred_image,''),COALESCE(a.server_lifetime_min_seconds,a.server_lifetime_seconds),COALESCE(a.server_lifetime_max_seconds,a.server_lifetime_seconds),a.auto_interval_seconds,a.auto_batch_size,a.build_spacing_minutes,a.build_spacing_max_minutes,a.auto_max_concurrent,a.server_lifetime_seconds,COALESCE(n.proxy_id::text,''),a.runtime_status,COALESCE(a.runtime_status_detail,''),a.desired_server_count,a.fallback_any_region,COALESCE((SELECT COALESCE((ps.canonical->'Capacity'->>'ComputeLimit')::int,(ps.data->'Limits'->>'DropletLimit')::int) FROM provider_snapshots ps WHERE ps.account_id=a.id ORDER BY ps.created_at DESC LIMIT 1),0),COALESCE((SELECT (ps.canonical->'Capacity'->>'LimitKnown')::boolean FROM provider_snapshots ps WHERE ps.account_id=a.id AND ps.canonical IS NOT NULL ORDER BY ps.created_at DESC LIMIT 1),true),COALESCE((SELECT COALESCE((ps.canonical->'Capacity'->>'ComputeInUse')::int,jsonb_array_length(COALESCE(ps.data->'Droplets','[]'::jsonb))) FROM provider_snapshots ps WHERE ps.account_id=a.id ORDER BY ps.created_at DESC LIMIT 1),0),(SELECT max(ps.created_at) FROM provider_snapshots ps WHERE ps.account_id=a.id),(SELECT max(d.created_at) FROM droplets d WHERE d.account_id=a.id AND d.state<>'DELETED'),(SELECT max(d.updated_at) FROM droplets d WHERE d.account_id=a.id AND d.state='DELETED'),(SELECT min(d.expires_at) FROM droplets d WHERE d.account_id=a.id AND d.state<>'DELETED' AND d.expires_at IS NOT NULL AND d.expires_at>now()),(SELECT old_limit FROM account_capacity_events ce WHERE ce.account_id=a.id ORDER BY ce.detected_at DESC LIMIT 1),(SELECT new_limit FROM account_capacity_events ce WHERE ce.account_id=a.id ORDER BY ce.detected_at DESC LIMIT 1),(SELECT delta FROM account_capacity_events ce WHERE ce.account_id=a.id ORDER BY ce.detected_at DESC LIMIT 1),(SELECT detected_at FROM account_capacity_events ce WHERE ce.account_id=a.id ORDER BY ce.detected_at DESC LIMIT 1),a.provider_state,COALESCE(a.provider_state_detail,''),COALESCE(a.provider_error_state,''),COALESCE(a.provider_error_detail,''),a.provider_state_at,a.provider_checked_at FROM accounts a LEFT JOIN network_profiles n ON n.account_id=a.id LEFT JOIN proxies p ON p.id=n.proxy_id WHERE a.deleted_at IS NULL OR a.runtime_status='DELETE_PENDING' ORDER BY a.created_at DESC`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "accounts_query_failed", "detail": err.Error()})
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, name, email, region, provider, mode, proxy, image, proxyID, runtimeStatus, runtimeDetail, providerStateDB, providerStateDetail, providerErrorState, providerErrorDetail string
		var regions, sizes, images []byte
		var interval, batch, spacingMinutes, spacingMaxMinutes, concurrent, lifetime, lifetimeMin, lifetimeMax, desired, dropletLimit, providerDroplets int
		var enabled, fallbackAnyRegion, limitKnown bool
		var created time.Time
		var capacityCheckedAt, providerStateAt, providerObservationAt, lastDropletCreated, lastDropletDeleted, nextDropletDelete sql.NullTime
		var capacityChangedAt sql.NullTime
		var capacityOld, capacityNew, capacityDelta sql.NullInt64
		if err := rows.Scan(&id, &name, &email, &region, &provider, &enabled, &created, &mode, &proxy, &regions, &sizes, &images, &image, &lifetimeMin, &lifetimeMax, &interval, &batch, &spacingMinutes, &spacingMaxMinutes, &concurrent, &lifetime, &proxyID, &runtimeStatus, &runtimeDetail, &desired, &fallbackAnyRegion, &dropletLimit, &limitKnown, &providerDroplets, &capacityCheckedAt, &lastDropletCreated, &lastDropletDeleted, &nextDropletDelete, &capacityOld, &capacityNew, &capacityDelta, &capacityChangedAt, &providerStateDB, &providerStateDetail, &providerErrorState, &providerErrorDetail, &providerStateAt, &providerObservationAt); err != nil {
			writeJSON(w, 500, map[string]string{"error": "accounts_scan_failed", "detail": err.Error()})
			return
		}
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
		var builtRegionsRaw []byte
		_ = s.DB.QueryRowContext(r.Context(), `SELECT COALESCE(jsonb_agg(region ORDER BY region),'[]'::jsonb) FROM (SELECT DISTINCT x->>'RegionID' region FROM (SELECT canonical FROM provider_snapshots WHERE account_id=$1 AND canonical IS NOT NULL ORDER BY created_at DESC LIMIT 1) ps CROSS JOIN LATERAL jsonb_array_elements(COALESCE(ps.canonical->'Inventory'->'Servers','[]'::jsonb)) x WHERE COALESCE(x->>'RegionID','')<>'') q`, id).Scan(&builtRegionsRaw)
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
		providerState := providerStateDB
		if providerState == "" {
			providerState = "UNKNOWN"
		}
		canCreate := enabled && providerState == "ACTIVE" && providerErrorState == "" && providerFreshness == "fresh" && capacityFreshness == "fresh" && (!limitKnown || dropletLimit > providerDroplets) && runtimeStatus == "READY"
		var providerMeta map[string]any
		_ = json.Unmarshal([]byte(providerStateDetail), &providerMeta)
		if !enabled {
			providerState = "DISABLED"
			canCreate = false
		} else if runtimeStatus != "READY" {
			canCreate = false
		}
		providerName := provider
		if provider == "digitalocean" {
			providerName = "DigitalOcean"
		} else if provider == "vultr" {
			providerName = "Vultr"
		}
		providerReason := "Ready to create servers"
		if !enabled {
			providerReason = "Account disabled in panel"
		} else if providerState == "LOCKED" {
			providerReason = providerName + " account is locked"
		} else if providerState == "TOKEN_INVALID" {
			providerReason = providerName + " credential is invalid"
		} else if providerState == "PERMISSION_DENIED" {
			providerReason = providerName + " permission denied"
		} else if providerState == "BILLING_BLOCKED" {
			providerReason = providerName + " billing blocked; update the provider billing profile"
		} else if providerState == "RATE_LIMITED" {
			providerReason = providerName + " rate limited"
		} else if providerErrorState != "" {
			providerReason = providerErrorState + ": " + providerErrorDetail
		} else if providerFreshness != "fresh" {
			providerReason = "Provider state is " + providerFreshness
		} else if capacityFreshness != "fresh" {
			providerReason = "Capacity snapshot is " + capacityFreshness
		} else if dropletLimit > 0 && providerDroplets >= dropletLimit {
			providerReason = "Droplet limit reached"
		} else if runtimeStatus != "READY" {
			providerReason = runtimeStatus
		}
		if v, ok := providerMeta["provider_error"].(string); ok && v != "" && providerState != "ACTIVE" {
			providerReason = v
		}
		var proxyIDs []string
		pr, _ := s.DB.QueryContext(r.Context(), `SELECT proxy_id::text FROM account_proxy_pool WHERE account_id=$1 AND enabled=true ORDER BY priority,proxy_id`, id)
		if pr != nil {
			for pr.Next() {
				var pid string
				if pr.Scan(&pid) == nil {
					proxyIDs = append(proxyIDs, pid)
				}
			}
			pr.Close()
		}
		var managedActive int
		_ = s.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM droplets WHERE account_id=$1 AND state<>'DELETED'`, id).Scan(&managedActive)
		desiredRemaining := max(0, desired-managedActive)
		providerCanCreate := canCreate
		schedulerCanBuild := providerCanCreate && desiredRemaining > 0
		schedulerReason := "Ready to build"
		if desiredRemaining <= 0 {
			schedulerReason = "Desired target reached"
		} else if !providerCanCreate {
			schedulerReason = providerReason
		}
		canCreate = schedulerCanBuild
		evidence := s.capacityEvidenceForAccount(id, provider)
		out = append(out, map[string]any{"id": id, "name": name, "email": email, "provider": provider, "region": region, "network": mode, "proxy": proxy, "enabled": enabled, "added_at": created.UTC().Format("2006-01-02 15:04:05"), "regions": json.RawMessage(regions), "built_regions": json.RawMessage(builtRegionsRaw), "sizes": json.RawMessage(sizes), "images": json.RawMessage(images), "image": image, "lifetime_min_seconds": lifetimeMin, "lifetime_max_seconds": lifetimeMax, "interval_seconds": interval, "batch_size": batch, "build_spacing_minutes": spacingMinutes, "build_spacing_max_minutes": spacingMaxMinutes, "max_concurrent": concurrent, "lifetime_seconds": lifetime, "proxy_id": proxyID, "proxy_ids": proxyIDs, "billing": s.accountBilling(r.Context(), id), "runtime_status": runtimeStatus, "runtime_status_detail": runtimeDetail, "desired_server_count": desired, "desired_remaining": desiredRemaining, "capacity_state": evidence.State, "capacity_lower_bound": evidence.LowerBound, "capacity_source": evidence.Source, "capacity_probe_in_flight": evidence.ProbeInFlight, "capacity_probe_after": evidence.ProbeAfter, "fallback_any_region": fallbackAnyRegion, "provider_state": providerState, "can_create": canCreate, "provider_can_create": providerCanCreate, "scheduler_can_build": schedulerCanBuild, "provider_reason": providerReason, "scheduler_reason": schedulerReason, "provider_checked_at": providerChecked, "provider_observed_at": providerObserved, "provider_freshness": providerFreshness, "provider_error_state": providerErrorState, "capacity_checked_at": capacityChecked, "capacity_freshness": capacityFreshness, "droplet_limit": dropletLimit, "capacity_limit_known": limitKnown, "provider_droplets": providerDroplets, "managed_servers": managedActive, "droplet_available": func() any {
			if !limitKnown {
				return nil
			}
			return max(0, dropletLimit-providerDroplets)
		}(), "last_droplet_created": ldc, "last_droplet_deleted": ldd, "next_droplet_delete": ndd, "capacity_old": co, "capacity_new": cn, "capacity_delta": cd, "capacity_changed_at": cca})
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
			continue
		}
		out = append(out, map[string]any{"id": id, "name": name, "type": typ, "host": host, "port": port, "status": status, "exit_ip": ip, "country": country, "asn": asn, "latency_ms": latency, "failure_count": fail, "last_checked_at": checked, "adapter": adapter})
	}
	writeJSON(w, 200, out)
}

func errorBody() map[string]string { return map[string]string{"error": "internal_error"} }
