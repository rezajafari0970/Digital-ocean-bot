package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
)

type accountDashboard struct {
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
	// Best-effort automatic identity refresh. Rate limiting is enforced by the
	// persisted identity timestamp so opening Details does not spam providers.
	go func(accountID string) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		s.refreshNetworkIdentityIfDue(ctx, accountID)
	}(id)
	var name, provider, email, externalID string
	var enabled bool
	if err := s.DB.QueryRowContext(r.Context(), `SELECT name,provider,enabled,COALESCE(email,''),COALESCE(external_id,'') FROM accounts WHERE id=$1`, id).Scan(&name, &provider, &enabled, &email, &externalID); err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, 404, map[string]string{"error": "not_found"})
		} else {
			writeJSON(w, 500, errorBody())
		}
		return
	}

	var latestAuditedBrowser string
	_ = s.DB.QueryRowContext(
		r.Context(),
		`SELECT COALESCE(runtime,'')
 FROM account_browser_identities
 WHERE account_id=$1
 ORDER BY checked_at DESC
 LIMIT 1`,
		id,
	).Scan(&latestAuditedBrowser)

	d := accountDashboard{Account: map[string]any{"id": id, "name": name, "provider": provider, "enabled": enabled, "email": email, "external_id": externalID}, Capacity: map[string]any{}, Resources: map[string]any{}, Network: map[string]any{}, Runtime: map[string]any{}, Deployments: map[string]any{}}
	var total, managed, active int
	_ = s.DB.QueryRowContext(r.Context(), `SELECT count(*),count(*) FILTER(WHERE managed),count(*) FILTER(WHERE state='active') FROM resources WHERE account_id=$1`, id).Scan(&total, &managed, &active)
	d.Resources = map[string]any{"total": total, "managed": managed, "unmanaged": total - managed, "active": active}

	var limit int
	_ = s.DB.QueryRowContext(r.Context(), `SELECT COALESCE((data->'Limits'->>'DropletLimit')::int,0) FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, id).Scan(&limit)
	available := limit - active
	if available < 0 {
		available = 0
	}
	var providerDroplets int
	var lastRefresh sql.NullTime
	_ = s.DB.QueryRowContext(r.Context(), `SELECT COALESCE(jsonb_array_length(data->'Droplets'),0),created_at FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, id).Scan(&providerDroplets, &lastRefresh)
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
	d.Capacity = map[string]any{"managed_droplets": managedDroplets, "last_managed_server_created": lastCreated.Time, "data_available": lastRefresh.Valid, "data_status": freshnessStatus, "snapshot_age_seconds": ageSeconds, "stale_after_seconds": 120}
	if lastRefresh.Valid {
		d.Capacity["droplet_limit"] = limit
		d.Capacity["provider_droplets"] = providerDroplets
		d.Capacity["available"] = providerAvailable
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
	var opKind, opState, opResource string
	var opAt sql.NullTime
	if err := s.DB.QueryRowContext(r.Context(), `SELECT kind,state,COALESCE(resource_id,''),updated_at FROM operations WHERE account_id=$1 AND kind IN ('CREATE_DROPLET','DELETE_DROPLET') ORDER BY updated_at DESC LIMIT 1`, id).Scan(&opKind, &opState, &opResource, &opAt); err == nil {
		d.Network["last_operation_kind"] = opKind
		d.Network["last_operation_state"] = opState
		d.Network["last_operation_resource"] = opResource
		d.Network["last_operation_at"] = opAt.Time
		d.Network["egress_fail_closed"] = opState == "unknown"
	}
	// Backfill identity from already-observed proxy data so existing accounts do
	// not stay unknown after this feature is introduced.
	if proxyID.Valid {
		_, _ = s.DB.ExecContext(r.Context(), `INSERT INTO account_network_identities(account_id,timezone,locale,exit_ip,subnet_key,asn,country) SELECT $1,'UTC','en-US',p.exit_ip,CASE WHEN family(p.exit_ip)=4 THEN host(network(set_masklen(p.exit_ip,24)))||'/24' ELSE host(network(set_masklen(p.exit_ip,48)))||'/48' END,COALESCE(p.asn,''),COALESCE(p.country,'') FROM proxies p WHERE p.id=$2 AND p.exit_ip IS NOT NULL ON CONFLICT(account_id) DO NOTHING`, id, proxyID.String)
		// Mark fallback identity as needing an explicit proxy test; do not perform
		// external geo requests merely because Details was opened.
		var fallback bool
		_ = s.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM account_network_identities WHERE account_id=$1 AND timezone='UTC')`, id).Scan(&fallback)
		if fallback {
			d.Network["identity_refresh_required"] = true
		}
	}
	var exitIP, subnet, asn, country, timezone, locale, preferredCountry, preferredCode, stickySession string
	var fallbackActive bool
	var lastHealth sql.NullTime
	var lastHealthOK sql.NullBool
	var rotationStarted sql.NullTime
	if err := s.DB.QueryRowContext(r.Context(), `SELECT COALESCE(host(exit_ip),''),COALESCE(subnet_key,''),COALESCE(asn,''),COALESCE(country,''),timezone,locale,COALESCE(preferred_country,''),COALESCE(preferred_country_code,''),COALESCE(sticky_session,''),fallback_active,last_health_at,last_health_ok,rotation_started_at FROM account_network_identities WHERE account_id=$1`, id).Scan(&exitIP, &subnet, &asn, &country, &timezone, &locale, &preferredCountry, &preferredCode, &stickySession, &fallbackActive, &lastHealth, &lastHealthOK, &rotationStarted); err == nil {
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

	// Browser Identity is observational only.
	// Values are never altered here to manufacture a different fingerprint.
	var (
		bProfile, bPlatform, bUA, bTimezone, bLanguage string
		bScreen, bCanvas, bWebGLVendor, bWebGLRenderer string
		bWebGL, bAudio, bRects, bFonts                 string
		bHC                                            int
		bMemory                                        float64
		bTouch, bWebRTCLeak                            bool
		bRuntime, bAuditStatus, bExpectedCountry       string
		bExpectedTimezone, bExpectedLocale, bGeo       string
		bTimezoneMatch, bLocaleMatch                   bool
		bChecked                                       sql.NullTime
	)

	if err := s.DB.QueryRowContext(
		r.Context(),
		`SELECT
profile_namespace,
COALESCE(platform,''),
COALESCE(user_agent,''),
COALESCE(timezone,''),
COALESCE(language,''),
COALESCE(screen,''),
COALESCE(hardware_concurrency,0),
COALESCE(device_memory,0),
COALESCE(touch_support,false),
COALESCE(canvas_hash,''),
COALESCE(webgl_vendor,''),
COALESCE(webgl_renderer,''),
COALESCE(webgl_hash,''),
COALESCE(audio_hash,''),
COALESCE(client_rects_hash,''),
COALESCE(fonts_hash,''),
webrtc_leak,
checked_at,
COALESCE(runtime,''),COALESCE(audit_status,'unknown'),COALESCE(expected_country,''),COALESCE(expected_timezone,''),COALESCE(expected_locale,''),COALESCE(timezone_match,false),COALESCE(locale_match,false),COALESCE(geo_consistency,'unknown')
 FROM account_browser_identities
 WHERE account_id=$1 ORDER BY checked_at DESC LIMIT 1`,
		id,
	).Scan(
		&bProfile,
		&bPlatform,
		&bUA,
		&bTimezone,
		&bLanguage,
		&bScreen,
		&bHC,
		&bMemory,
		&bTouch,
		&bCanvas,
		&bWebGLVendor,
		&bWebGLRenderer,
		&bWebGL,
		&bAudio,
		&bRects,
		&bFonts,
		&bWebRTCLeak,
		&bChecked,
		&bRuntime, &bAuditStatus, &bExpectedCountry, &bExpectedTimezone, &bExpectedLocale, &bTimezoneMatch, &bLocaleMatch, &bGeo,
	); err == nil {

		shared := map[string]int{}

		checks := map[string]string{
			"canvas":       bCanvas,
			"webgl":        bWebGL,
			"audio":        bAudio,
			"client_rects": bRects,
			"fonts":        bFonts,
			"screen":       bScreen,
			"platform":     bPlatform,
		}

		for key, value := range checks {
			if value == "" {
				continue
			}

			var count int

			query := ""

			switch key {
			case "canvas":
				query = `SELECT count(*) FROM account_browser_identities
         WHERE account_id<>$1 AND canvas_hash=$2`
			case "webgl":
				query = `SELECT count(*) FROM account_browser_identities
         WHERE account_id<>$1 AND webgl_hash=$2`
			case "audio":
				query = `SELECT count(*) FROM account_browser_identities
         WHERE account_id<>$1 AND audio_hash=$2`
			case "client_rects":
				query = `SELECT count(*) FROM account_browser_identities
         WHERE account_id<>$1 AND client_rects_hash=$2`
			case "fonts":
				query = `SELECT count(*) FROM account_browser_identities
         WHERE account_id<>$1 AND fonts_hash=$2`
			case "screen":
				query = `SELECT count(*) FROM account_browser_identities
         WHERE account_id<>$1 AND screen=$2`
			case "platform":
				query = `SELECT count(*) FROM account_browser_identities
         WHERE account_id<>$1 AND platform=$2`
			}

			if query != "" {
				_ = s.DB.QueryRowContext(
					r.Context(),
					query,
					id,
					value,
				).Scan(&count)

				shared[key] = count
			}
		}

		d.Runtime["browser_identity"] = map[string]any{
			"profile_namespace":    bProfile,
			"platform":             bPlatform,
			"user_agent":           bUA,
			"timezone":             bTimezone,
			"language":             bLanguage,
			"screen":               bScreen,
			"hardware_concurrency": bHC,
			"device_memory":        bMemory,
			"touch_support":        bTouch,
			"canvas_hash":          bCanvas,
			"webgl_vendor":         bWebGLVendor,
			"webgl_renderer":       bWebGLRenderer,
			"webgl_hash":           bWebGL,
			"audio_hash":           bAudio,
			"client_rects_hash":    bRects,
			"fonts_hash":           bFonts,
			"webrtc_leak":          bWebRTCLeak,
			"checked_at":           bChecked.Time,
			"runtime":              bRuntime,
			"audit_status":         bAuditStatus,
			"expected_country":     bExpectedCountry,
			"expected_timezone":    bExpectedTimezone,
			"expected_locale":      bExpectedLocale,
			"timezone_match":       bTimezoneMatch,
			"locale_match":         bLocaleMatch,
			"geo_consistency":      bGeo,
			"shared":               shared,
		}
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
	var running, ready, failed int
	_ = s.DB.QueryRowContext(r.Context(), `SELECT count(*) FILTER(WHERE state NOT IN ('READY','FAILED')),count(*) FILTER(WHERE state='READY'),count(*) FILTER(WHERE state='FAILED') FROM deployments WHERE account_id=$1`, id).Scan(&running, &ready, &failed)
	d.Deployments = map[string]any{"active_now": running, "ready_now": ready, "historical_failures": failed}
	rowsFail, _ := s.DB.QueryContext(r.Context(), `SELECT id::text,current_step,attempt,COALESCE(last_error,''),updated_at FROM deployments WHERE account_id=$1 AND state='FAILED' ORDER BY updated_at DESC LIMIT 10`, id)
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
	if s.DB.QueryRowContext(r.Context(), `SELECT data FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, id).Scan(&snap) == nil {
		var disc digitalocean.DiscoveryResult
		if json.Unmarshal(snap, &disc) == nil {
			managedIDs := map[string]bool{}
			rows, _ := s.DB.QueryContext(r.Context(), `SELECT provider_resource_id FROM droplets WHERE account_id=$1 AND state <> 'DELETED'`, id)
			if rows != nil {
				defer rows.Close()
				for rows.Next() {
					var pid string
					if rows.Scan(&pid) == nil {
						managedIDs[pid] = true
					}
				}
			}
			for _, x := range disc.Droplets {
				ip := x.PublicIPv4
				if ip == "" {
					for _, n := range x.Networks.V4 {
						if n.Type == "public" {
							ip = n.IPAddress
							break
						}
					}
				}
				created, _ := time.Parse(time.RFC3339, x.CreatedAt)
				age := int64(0)
				if !created.IsZero() {
					age = int64(time.Since(created).Seconds())
				}
				managed := managedIDs[strconv.Itoa(x.ID)]
				d.Droplets = append(d.Droplets, map[string]any{"id": x.ID, "name": x.Name, "ip": ip, "region": x.Region.Slug, "status": x.Status, "created_at": x.CreatedAt, "age_seconds": age, "managed": managed, "ownership": map[bool]string{true: "managed", false: "foreign"}[managed], "tags": x.Tags})
			}
		}
	}
	writeJSON(w, 200, d)
}
