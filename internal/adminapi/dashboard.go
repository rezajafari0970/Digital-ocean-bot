package adminapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

type accountDashboard struct {
	Billing     json.RawMessage  `json:"billing"`
	Account     map[string]any   `json:"account"`
	Capacity    map[string]any   `json:"capacity"`
	Resources   map[string]any   `json:"resources"`
	Network     map[string]any   `json:"network"`
	Runtime     map[string]any   `json:"runtime"`
	Deployments map[string]any   `json:"deployments"`
	Droplets    []map[string]any `json:"droplets"`
}

func (s *Server) accountDashboard(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var name, provider, email, externalID, runtimeStatus, runtimeDetail string
	var enabled bool
	if err := s.DB.QueryRowContext(r.Context(), `SELECT name,provider,enabled,COALESCE(email,''),COALESCE(external_id,''),runtime_status,COALESCE(runtime_status_detail,'') FROM accounts WHERE id=$1`, id).Scan(&name, &provider, &enabled, &email, &externalID, &runtimeStatus, &runtimeDetail); err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, 404, map[string]string{"error": "not_found"})
		} else {
			writeJSON(w, 500, errorBody())
		}
		return
	}

	build, buildErr := s.accountBuildState(r.Context(), id)
	if buildErr != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	d := accountDashboard{Account: map[string]any{"id": id, "name": name, "provider": provider, "enabled": enabled, "email": email, "external_id": externalID, "runtime_status": runtimeStatus, "provider_state": build.State, "provider_reason": build.Reason, "can_create": build.SchedulerCanBuild, "provider_can_create": build.ProviderCanCreate, "scheduler_can_build": build.SchedulerCanBuild, "scheduler_reason": build.SchedulerReason, "create_block": build.Block}, Capacity: map[string]any{}, Resources: map[string]any{}, Network: map[string]any{}, Runtime: map[string]any{}, Deployments: map[string]any{}}
	d.Billing = s.accountBilling(r.Context(), id)
	var total, managed, active int
	_ = s.DB.QueryRowContext(r.Context(), `SELECT count(*) FILTER(WHERE state<>'deleted'),count(*) FILTER(WHERE state<>'deleted' AND managed),count(*) FILTER(WHERE state='active') FROM resources WHERE account_id=$1 AND type='server'`, id).Scan(&total, &managed, &active)
	d.Resources = map[string]any{"total": total, "managed": managed, "unmanaged": total - managed, "active": active}

	limit, limitKnown, providerDroplets := build.Limit, build.LimitKnown, build.InUse
	var lastRefresh sql.NullTime
	_ = s.DB.QueryRowContext(r.Context(), "SELECT created_at FROM provider_snapshots WHERE account_id=$1 AND canonical IS NOT NULL ORDER BY created_at DESC LIMIT 1", id).Scan(&lastRefresh)
	var lastCreated sql.NullTime
	_ = s.DB.QueryRowContext(r.Context(), `SELECT max(created_at) FROM droplets WHERE account_id=$1`, id).Scan(&lastCreated)
	var managedDroplets int
	_ = s.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM droplets WHERE account_id=$1 AND state <> 'DELETED'`, id).Scan(&managedDroplets)
	providerAvailable := limit - providerDroplets
	if providerAvailable < 0 {
		providerAvailable = 0
	}
	freshnessStatus := "unavailable"
	ageSeconds := int64(0)
	if lastRefresh.Valid {
		ageSeconds = int64(time.Since(lastRefresh.Time).Seconds())
		if ageSeconds <= 120 {
			freshnessStatus = "fresh"
		} else {
			freshnessStatus = "stale"
		}
	}
	var desiredServers int
	_ = s.DB.QueryRowContext(r.Context(), `SELECT desired_server_count FROM accounts WHERE id=$1`, id).Scan(&desiredServers)
	desiredRemaining := build.DesiredRemaining
	if desiredRemaining < 0 {
		desiredRemaining = 0
	}
	evidence := s.capacityEvidenceForAccount(id, provider)
	d.Capacity = map[string]any{"managed_servers": managedDroplets, "managed_droplets": managedDroplets, "desired_servers": desiredServers, "desired_remaining": desiredRemaining, "capacity_state": evidence.State, "buildable_now": build.Buildable, "pending_builds": build.Pending, "plan_available": build.PlanAvailable, "lower_bound": evidence.LowerBound, "evidence_source": evidence.Source, "probe_in_flight": evidence.ProbeInFlight, "probe_after": evidence.ProbeAfter, "last_managed_server_created": lastCreated.Time, "data_available": lastRefresh.Valid, "data_status": freshnessStatus, "snapshot_age_seconds": ageSeconds, "stale_after_seconds": 120}
	if lastRefresh.Valid {
		d.Capacity["limit_known"] = limitKnown
		d.Capacity["provider_servers"] = providerDroplets
		d.Capacity["provider_droplets"] = providerDroplets
		if limitKnown {
			d.Capacity["server_limit"] = limit
			d.Capacity["droplet_limit"] = limit
			d.Capacity["available"] = providerAvailable
		} else {
			d.Capacity["server_limit"] = nil
			d.Capacity["droplet_limit"] = nil
			d.Capacity["available"] = nil
		}
		d.Capacity["last_refresh"] = lastRefresh.Time
	} else {
		d.Capacity["droplet_limit"] = nil
		d.Capacity["provider_droplets"] = nil
		d.Capacity["available"] = nil
		d.Capacity["last_refresh"] = nil
	}
	var mode, status string
	var proxyID sql.NullString
	_ = s.DB.QueryRowContext(r.Context(), `SELECT n.mode,n.proxy_id::text,COALESCE(p.status,'') FROM network_profiles n LEFT JOIN proxies p ON p.id=n.proxy_id WHERE n.account_id=$1`, id).Scan(&mode, &proxyID, &status)
	d.Network = map[string]any{"mode": mode, "proxy_id": proxyID.String, "proxy_status": status, "isolation_status": "unknown"}
	var proxyAdapter string
	_ = s.DB.QueryRowContext(r.Context(), `SELECT COALESCE(p.adapter,'generic') FROM network_profiles n LEFT JOIN proxies p ON p.id=n.proxy_id WHERE n.account_id=$1`, id).Scan(&proxyAdapter)
	d.Network["egress_mode"] = proxyAdapter
	var poolSize, healthyPool int
	_ = s.DB.QueryRowContext(r.Context(), `SELECT count(*) FILTER(WHERE ap.enabled),count(*) FILTER(WHERE ap.enabled AND p.status='healthy') FROM account_proxy_pool ap JOIN proxies p ON p.id=ap.proxy_id WHERE ap.account_id=$1`, id).Scan(&poolSize, &healthyPool)
	d.Network["proxy_pool_size"] = poolSize
	d.Network["healthy_proxy_count"] = healthyPool
	var opKind, opState, opResource string
	var opAt sql.NullTime
	if err := s.DB.QueryRowContext(r.Context(), `SELECT kind,state,COALESCE(resource_id,''),updated_at FROM operations WHERE account_id=$1 AND kind IN ('CREATE_DROPLET','DELETE_DROPLET') ORDER BY updated_at DESC LIMIT 1`, id).Scan(&opKind, &opState, &opResource, &opAt); err == nil {
		displayKind := opKind
		if opKind == "CREATE_DROPLET" {
			displayKind = "CREATE_SERVER"
		}
		if opKind == "DELETE_DROPLET" {
			displayKind = "DELETE_SERVER"
		}
		d.Network["last_operation_kind"] = displayKind
		d.Network["last_operation_state"] = opState
		d.Network["last_operation_resource"] = opResource
		d.Network["last_operation_at"] = opAt.Time
		d.Network["egress_fail_closed"] = opState == "unknown"
	}
	// Dashboard is strictly read-only. Proxy identity is owned and refreshed by
	// the shared Proxy Control Plane worker, never by UI reads.
	var exitIP, subnet, asn, country, countryCode, timezone, locale, preferredCountry, preferredCode, stickySession string
	var fallbackActive bool
	var lastHealth sql.NullTime
	var lastHealthOK sql.NullBool
	var rotationStarted sql.NullTime
	if err := s.DB.QueryRowContext(r.Context(), `SELECT COALESCE(host(exit_ip),''),COALESCE(subnet_key,''),COALESCE(asn,''),COALESCE(country,''),COALESCE(country_code,''),timezone,locale,COALESCE(preferred_country,''),COALESCE(preferred_country_code,''),COALESCE(sticky_session,''),fallback_active,last_health_at,last_health_ok,rotation_started_at FROM account_network_identities WHERE account_id=$1`, id).Scan(&exitIP, &subnet, &asn, &country, &countryCode, &timezone, &locale, &preferredCountry, &preferredCode, &stickySession, &fallbackActive, &lastHealth, &lastHealthOK, &rotationStarted); err == nil {
		isolation := "isolated"
		var collision bool
		_ = s.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM account_network_identities WHERE account_id<>$1 AND ((exit_ip IS NOT NULL AND exit_ip=$2::inet) OR (subnet_key<>'' AND subnet_key=$3)))`, id, exitIP, subnet).Scan(&collision)
		if collision {
			isolation = "collision"
		}
		d.Network["exit_ip"] = exitIP
		d.Network["subnet"] = subnet
		d.Network["asn"] = asn
		d.Network["country"] = country
		d.Network["country_code"] = countryCode
		d.Network["timezone"] = timezone
		d.Network["locale"] = locale
		d.Network["isolation_status"] = isolation
		d.Network["preferred_country"] = preferredCountry
		d.Network["preferred_country_code"] = preferredCode
		d.Network["sticky_session"] = stickySession
		d.Network["fallback_active"] = fallbackActive
		d.Network["last_health_at"] = lastHealth.Time
		d.Network["last_health_ok"] = lastHealthOK.Valid && lastHealthOK.Bool
		d.Network["rotation_started_at"] = rotationStarted.Time
		d.Network["sticky_ttl_seconds"] = 1800
	}

	var circuit string
	var failures int
	var retry sql.NullTime
	_ = s.DB.QueryRowContext(r.Context(), `SELECT circuit_state,consecutive_failures,retry_after FROM account_runtime_state WHERE account_id=$1`, id).Scan(&circuit, &failures, &retry)
	if d.Runtime == nil {
		d.Runtime = map[string]any{}
	}
	d.Runtime["circuit"] = circuit
	d.Runtime["failures"] = failures
	d.Runtime["retry_after"] = retry.Time
	var running, ready int
	_ = s.DB.QueryRowContext(r.Context(), `SELECT count(*) FILTER(WHERE state NOT IN ('READY','FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK','PANEL_COMPLETE')),count(*) FILTER(WHERE state='READY') FROM deployments WHERE account_id=$1`, id).Scan(&running, &ready)
	d.Deployments = map[string]any{"active_now": running, "ready_now": ready}
	rowsFail, _ := s.DB.QueryContext(r.Context(), `SELECT id::text,current_step,attempt,COALESCE(last_error,''),updated_at FROM deployments WHERE account_id=$1 AND state='FAILED' AND updated_at > now()-interval '24 hours' ORDER BY updated_at DESC LIMIT 10`, id)
	if rowsFail != nil {
		defer rowsFail.Close()
		history := []map[string]any{}
		for rowsFail.Next() {
			var did, step, msg string
			var attempt int
			var at time.Time
			if rowsFail.Scan(&did, &step, &attempt, &msg, &at) == nil {
				history = append(history, map[string]any{"id": did, "step": step, "attempt": attempt, "error": msg, "updated_at": at})
			}
		}
		d.Deployments["failure_history"] = history
	}
	var snap []byte
	if s.DB.QueryRowContext(r.Context(), `SELECT canonical FROM provider_snapshots WHERE account_id=$1 AND canonical IS NOT NULL ORDER BY created_at DESC LIMIT 1`, id).Scan(&snap) == nil {
		var obs providers.Observation
		if json.Unmarshal(snap, &obs) == nil {
			type managedServerInfo struct {
				State     string
				ExpiresAt sql.NullTime
			}
			managedIDs := map[string]managedServerInfo{}
			rows, _ := s.DB.QueryContext(r.Context(), `SELECT provider_resource_id,state,expires_at FROM droplets WHERE account_id=$1 AND state <> 'DELETED'`, id)
			if rows != nil {
				defer rows.Close()
				for rows.Next() {
					var pid, lifecycleState string
					var expiresAt sql.NullTime
					if rows.Scan(&pid, &lifecycleState, &expiresAt) == nil {
						managedIDs[pid] = managedServerInfo{State: lifecycleState, ExpiresAt: expiresAt}
					}
				}
			}
			for _, x := range obs.Inventory.Servers {
				age := int64(0)
				if !x.CreatedAt.IsZero() {
					age = int64(time.Since(x.CreatedAt).Seconds())
				}
				info, managed := managedIDs[x.ID]
				var expiresAt any
				if info.ExpiresAt.Valid {
					expiresAt = info.ExpiresAt.Time
				}
				d.Droplets = append(d.Droplets, map[string]any{"id": x.ID, "name": x.Name, "provider": provider, "ip": x.PrimaryIPv4, "region": x.RegionID, "status": x.State, "lifecycle_state": info.State, "expires_at": expiresAt, "created_at": x.CreatedAt, "age_seconds": age, "managed": managed, "ownership": map[bool]string{true: "managed", false: "foreign"}[managed], "tags": x.Tags})
			}
		}
	}
	writeJSON(w, 200, d)
}
