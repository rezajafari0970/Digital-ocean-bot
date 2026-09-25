package adminapi

import (
	"encoding/json"
	"net/http"
	"strings"
)

type browserIdentityInput struct {
	Platform            string   `json:"platform"`
	UserAgent           string   `json:"user_agent"`
	Timezone            string   `json:"timezone"`
	Language            string   `json:"language"`
	Screen              string   `json:"screen"`
	HardwareConcurrency int      `json:"hardware_concurrency"`
	DeviceMemory        float64  `json:"device_memory"`
	TouchSupport        bool     `json:"touch_support"`
	CanvasHash          string   `json:"canvas_hash"`
	WebGLVendor         string   `json:"webgl_vendor"`
	WebGLRenderer       string   `json:"webgl_renderer"`
	WebGLHash           string   `json:"webgl_hash"`
	AudioHash           string   `json:"audio_hash"`
	ClientRectsHash     string   `json:"client_rects_hash"`
	FontsHash           string   `json:"fonts_hash"`
	WebRTCCandidates    []string `json:"webrtc_candidates"`
}

func (s *Server) browserIdentity(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var x browserIdentityInput
	if json.NewDecoder(r.Body).Decode(&x) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	raw, _ := json.Marshal(x)
	cand, _ := json.Marshal(x.WebRTCCandidates)
	var exitIP string
	_ = s.DB.QueryRowContext(r.Context(), `SELECT COALESCE(host(exit_ip),'') FROM account_network_identities WHERE account_id=$1`, id).Scan(&exitIP)
	leak := false
	for _, c := range x.WebRTCCandidates {
		lc := strings.ToLower(c)
		if strings.Contains(lc, " typ host ") || strings.Contains(lc, " typ srflx ") {
			if exitIP == "" || !strings.Contains(c, exitIP) {
				leak = true
			}
		}
	}
	ns := "browser:" + strings.ReplaceAll(id, "-", "")
	_, err := s.DB.ExecContext(r.Context(), `INSERT INTO account_browser_identities(account_id,profile_namespace,platform,user_agent,timezone,language,screen,hardware_concurrency,device_memory,touch_support,canvas_hash,webgl_vendor,webgl_renderer,webgl_hash,audio_hash,client_rects_hash,fonts_hash,webrtc_candidates,webrtc_leak,raw,checked_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,now()) ON CONFLICT(account_id) DO UPDATE SET profile_namespace=EXCLUDED.profile_namespace,platform=EXCLUDED.platform,user_agent=EXCLUDED.user_agent,timezone=EXCLUDED.timezone,language=EXCLUDED.language,screen=EXCLUDED.screen,hardware_concurrency=EXCLUDED.hardware_concurrency,device_memory=EXCLUDED.device_memory,touch_support=EXCLUDED.touch_support,canvas_hash=EXCLUDED.canvas_hash,webgl_vendor=EXCLUDED.webgl_vendor,webgl_renderer=EXCLUDED.webgl_renderer,webgl_hash=EXCLUDED.webgl_hash,audio_hash=EXCLUDED.audio_hash,client_rects_hash=EXCLUDED.client_rects_hash,fonts_hash=EXCLUDED.fonts_hash,webrtc_candidates=EXCLUDED.webrtc_candidates,webrtc_leak=EXCLUDED.webrtc_leak,raw=EXCLUDED.raw,checked_at=now()`, id, ns, x.Platform, x.UserAgent, x.Timezone, x.Language, x.Screen, x.HardwareConcurrency, x.DeviceMemory, x.TouchSupport, x.CanvasHash, x.WebGLVendor, x.WebGLRenderer, x.WebGLHash, x.AudioHash, x.ClientRectsHash, x.FontsHash, cand, leak, raw)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "browser_identity_store_failed", "detail": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"stored": true, "webrtc_leak": leak})
}
