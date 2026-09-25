package adminapi

import (
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

type geoBrowserAudit struct {
	Runtime        string               `json:"runtime"`
	RuntimeVersion string               `json:"runtime_version"`
	RuntimeEngine  string               `json:"runtime_engine"`
	Identity       browserIdentityInput `json:"identity"`
}

func (s *Server) runGeoBrowserAudit(
	w http.ResponseWriter,
	r *http.Request,
) {
	id := r.PathValue("id")

	var (
		runtime  string
		country  string
		timezone string
		locale   string
		exitIP   string
	)

	err := s.DB.QueryRowContext(
		r.Context(),
		`SELECT
COALESCE(a.assigned_browser,''),
COALESCE(i.country,''),
COALESCE(NULLIF(i.timezone,''),'UTC'),
COALESCE(NULLIF(i.locale,''),'en-US'),
COALESCE(host(i.exit_ip),'')
 FROM accounts a
 JOIN account_network_identities i
   ON i.account_id=a.id
 WHERE a.id=$1`,
		id,
	).Scan(
		&runtime,
		&country,
		&timezone,
		&locale,
		&exitIP,
	)

	if err != nil || runtime == "" {
		writeJSON(
			w,
			http.StatusConflict,
			map[string]string{
				"error": "browser_geo_not_ready",
			},
		)
		return
	}

	ctx, cancel :=
		context.WithTimeout(
			r.Context(),
			30*time.Second,
		)

	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		"/opt/digital-ocean-bot/browser/run-geo-audit.sh",
		id,
		runtime,
		timezone,
		locale,
	)

	out, err := cmd.Output()

	if err != nil {
		writeJSON(
			w,
			http.StatusBadGateway,
			map[string]string{
				"error": "browser_geo_audit_failed",
			},
		)
		return
	}

	var x geoBrowserAudit

	if json.Unmarshal(out, &x) != nil {
		writeJSON(
			w,
			http.StatusBadGateway,
			map[string]string{
				"error": "invalid_browser_geo_audit",
			},
		)
		return
	}

	timezoneMatch :=
		strings.EqualFold(
			strings.TrimSpace(x.Identity.Timezone),
			strings.TrimSpace(timezone),
		)

	// Language may contain additional preference subtags.
	observedLanguage :=
		strings.ToLower(
			strings.TrimSpace(
				x.Identity.Language,
			),
		)

	expectedLocale :=
		strings.ToLower(
			strings.TrimSpace(locale),
		)

	localeMatch :=
		observedLanguage == expectedLocale ||
			strings.HasPrefix(
				observedLanguage,
				expectedLocale+"-",
			)

	geoStatus := "match"

	if !timezoneMatch || !localeMatch {
		geoStatus = "mismatch"
	}

	candidates, _ :=
		json.Marshal(
			x.Identity.WebRTCCandidates,
		)

	raw, _ :=
		json.Marshal(x.Identity)

	webrtcLeak := false

	for _, candidate := range x.Identity.WebRTCCandidates {
		lc := strings.ToLower(candidate)

		// mDNS .local host candidates do not expose a literal IP.
		if strings.Contains(lc, " typ host ") &&
			!strings.Contains(lc, ".local ") {
			webrtcLeak = true
		}

		// A server-reflexive address must agree with the
		// account's currently observed proxy exit IP.
		if strings.Contains(lc, " typ srflx ") &&
			(exitIP == "" || !strings.Contains(candidate, exitIP)) {
			webrtcLeak = true
		}
	}
	namespace :=
		"browser:" +
			strings.ReplaceAll(
				id,
				"-",
				"",
			) +
			":" +
			runtime

	_, err =
		s.DB.ExecContext(
			r.Context(),
			`INSERT INTO account_browser_identities(
account_id,
runtime,
profile_namespace,
platform,
user_agent,
timezone,
language,
screen,
hardware_concurrency,
device_memory,
touch_support,
canvas_hash,
webgl_vendor,
webgl_renderer,
webgl_hash,
audio_hash,
client_rects_hash,
fonts_hash,
webrtc_candidates,
webrtc_leak,
raw,
checked_at,
audit_status,
runtime_version,
runtime_engine,
expected_country,
expected_timezone,
expected_locale,
timezone_match,
locale_match,
geo_consistency
)
VALUES(
$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,
$11,$12,$13,$14,$15,$16,$17,$18,
$19,$20,$21,now(),'pass',$22,$23,
$24,$25,$26,$27,$28,$29
)
ON CONFLICT(account_id,runtime)
DO UPDATE SET
profile_namespace=EXCLUDED.profile_namespace,
platform=EXCLUDED.platform,
user_agent=EXCLUDED.user_agent,
timezone=EXCLUDED.timezone,
language=EXCLUDED.language,
screen=EXCLUDED.screen,
hardware_concurrency=EXCLUDED.hardware_concurrency,
device_memory=EXCLUDED.device_memory,
touch_support=EXCLUDED.touch_support,
canvas_hash=EXCLUDED.canvas_hash,
webgl_vendor=EXCLUDED.webgl_vendor,
webgl_renderer=EXCLUDED.webgl_renderer,
webgl_hash=EXCLUDED.webgl_hash,
audio_hash=EXCLUDED.audio_hash,
client_rects_hash=EXCLUDED.client_rects_hash,
fonts_hash=EXCLUDED.fonts_hash,
webrtc_candidates=EXCLUDED.webrtc_candidates,
webrtc_leak=EXCLUDED.webrtc_leak,
raw=EXCLUDED.raw,
checked_at=now(),
audit_status='pass',
audit_error=NULL,
runtime_version=EXCLUDED.runtime_version,
runtime_engine=EXCLUDED.runtime_engine,
expected_country=EXCLUDED.expected_country,
expected_timezone=EXCLUDED.expected_timezone,
expected_locale=EXCLUDED.expected_locale,
timezone_match=EXCLUDED.timezone_match,
locale_match=EXCLUDED.locale_match,
geo_consistency=EXCLUDED.geo_consistency`,
			id,
			runtime,
			namespace,
			x.Identity.Platform,
			x.Identity.UserAgent,
			x.Identity.Timezone,
			x.Identity.Language,
			x.Identity.Screen,
			x.Identity.HardwareConcurrency,
			x.Identity.DeviceMemory,
			x.Identity.TouchSupport,
			x.Identity.CanvasHash,
			x.Identity.WebGLVendor,
			x.Identity.WebGLRenderer,
			x.Identity.WebGLHash,
			x.Identity.AudioHash,
			x.Identity.ClientRectsHash,
			x.Identity.FontsHash,
			candidates,
			webrtcLeak,
			raw,
			x.RuntimeVersion,
			x.RuntimeEngine,
			country,
			timezone,
			locale,
			timezoneMatch,
			localeMatch,
			geoStatus,
		)

	if err != nil {
		writeJSON(
			w,
			http.StatusInternalServerError,
			map[string]string{
				"error": "browser_geo_store_failed",
			},
		)
		return
	}

	writeJSON(
		w,
		http.StatusOK,
		map[string]any{
			"runtime":         runtime,
			"country":         country,
			"exit_ip":         exitIP,
			"timezone":        timezone,
			"locale":          locale,
			"timezone_match":  timezoneMatch,
			"locale_match":    localeMatch,
			"geo_consistency": geoStatus,
			"webrtc_leak":     webrtcLeak,
		},
	)
}
