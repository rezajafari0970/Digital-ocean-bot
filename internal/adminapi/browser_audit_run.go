package adminapi

import (
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

type browserAuditResult struct {
	Runtime        string               `json:"runtime"`
	RuntimeVersion string               `json:"runtime_version"`
	RuntimeEngine  string               `json:"runtime_engine"`
	Identity       browserIdentityInput `json:"identity"`
}

func (s *Server) runAssignedBrowserAudit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var runtime string
	if err := s.DB.QueryRowContext(r.Context(), `SELECT COALESCE(assigned_browser,'') FROM accounts WHERE id=$1`, id).Scan(&runtime); err != nil || runtime == "" {
		writeJSON(w, 409, map[string]string{"error": "assigned_browser_missing"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/node", "/opt/digital-ocean-bot/browser/assigned-browser-audit.js", id, runtime)
	out, err := cmd.Output()
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "browser_audit_failed"})
		return
	}
	var x browserAuditResult
	if json.Unmarshal(out, &x) != nil {
		writeJSON(w, 502, map[string]string{"error": "invalid_browser_audit"})
		return
	}
	raw, _ := json.Marshal(x.Identity)
	cand, _ := json.Marshal(x.Identity.WebRTCCandidates)
	var exitIP string
	_ = s.DB.QueryRowContext(r.Context(), `SELECT COALESCE(host(exit_ip),'') FROM account_network_identities WHERE account_id=$1`, id).Scan(&exitIP)
	leak := false
	for _, c := range x.Identity.WebRTCCandidates {
		lc := strings.ToLower(c)
		if strings.Contains(lc, " typ host ") || strings.Contains(lc, " typ srflx ") {
			if exitIP == "" || !strings.Contains(c, exitIP) {
				leak = true
			}
		}
	}
	ns := "browser:" + strings.ReplaceAll(id, "-", "") + ":" + runtime
	_, err = s.DB.ExecContext(r.Context(), `INSERT INTO account_browser_identities(account_id,runtime,profile_namespace,platform,user_agent,timezone,language,screen,hardware_concurrency,device_memory,touch_support,canvas_hash,webgl_vendor,webgl_renderer,webgl_hash,audio_hash,client_rects_hash,fonts_hash,webrtc_candidates,webrtc_leak,raw,checked_at,audit_status,audit_error,runtime_version,runtime_engine) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,now(),'pass',NULL,$22,$23) ON CONFLICT(account_id,runtime) DO UPDATE SET profile_namespace=EXCLUDED.profile_namespace,platform=EXCLUDED.platform,user_agent=EXCLUDED.user_agent,timezone=EXCLUDED.timezone,language=EXCLUDED.language,screen=EXCLUDED.screen,hardware_concurrency=EXCLUDED.hardware_concurrency,device_memory=EXCLUDED.device_memory,touch_support=EXCLUDED.touch_support,canvas_hash=EXCLUDED.canvas_hash,webgl_vendor=EXCLUDED.webgl_vendor,webgl_renderer=EXCLUDED.webgl_renderer,webgl_hash=EXCLUDED.webgl_hash,audio_hash=EXCLUDED.audio_hash,client_rects_hash=EXCLUDED.client_rects_hash,fonts_hash=EXCLUDED.fonts_hash,webrtc_candidates=EXCLUDED.webrtc_candidates,webrtc_leak=EXCLUDED.webrtc_leak,raw=EXCLUDED.raw,checked_at=now(),audit_status='pass',audit_error=NULL,runtime_version=EXCLUDED.runtime_version,runtime_engine=EXCLUDED.runtime_engine`, id, runtime, ns, x.Identity.Platform, x.Identity.UserAgent, x.Identity.Timezone, x.Identity.Language, x.Identity.Screen, x.Identity.HardwareConcurrency, x.Identity.DeviceMemory, x.Identity.TouchSupport, x.Identity.CanvasHash, x.Identity.WebGLVendor, x.Identity.WebGLRenderer, x.Identity.WebGLHash, x.Identity.AudioHash, x.Identity.ClientRectsHash, x.Identity.FontsHash, cand, leak, raw, x.RuntimeVersion, x.RuntimeEngine)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "browser_audit_store_failed"})
		return
	}
	writeJSON(w, 200, map[string]any{"runtime": runtime, "status": "pass", "webrtc_leak": leak})
}
