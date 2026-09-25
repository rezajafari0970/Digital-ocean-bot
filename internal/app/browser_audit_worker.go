package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/geoctx"
	"os"
	"os/exec"
	"strings"
	"time"
)

type auditIdentity struct {
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
type auditOutput struct {
	Runtime        string        `json:"runtime"`
	RuntimeVersion string        `json:"runtime_version"`
	RuntimeEngine  string        `json:"runtime_engine"`
	Identity       auditIdentity `json:"identity"`
}

func (c Container) RunBrowserAuditQueue(ctx context.Context) {
	rows, err := c.DB.QueryContext(ctx, `SELECT q.account_id::text,COALESCE(a.assigned_browser,''),q.attempts FROM browser_audit_queue q JOIN accounts a ON a.id=q.account_id WHERE q.not_before<=now() ORDER BY q.requested_at LIMIT 4`)
	if err != nil {
		return
	}
	defer rows.Close()
	type job struct {
		id, rt   string
		attempts int
	}
	var jobs []job
	for rows.Next() {
		var j job
		if rows.Scan(&j.id, &j.rt, &j.attempts) == nil {
			jobs = append(jobs, j)
		}
	}
	for _, j := range jobs {
		if j.rt == "" {
			c.auditRetry(ctx, j.id, j.attempts, "assigned browser missing")
			continue
		}
		g, err := geoctx.ForAccount(ctx, c.DB, j.id)
		if err != nil {
			c.auditRetry(ctx, j.id, j.attempts, err.Error())
			continue
		}
		runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		cmd := exec.CommandContext(runCtx, "/opt/digital-ocean-bot/browser/run-geo-audit.sh", j.id, j.rt, g.Timezone, g.Locale)
		cmd.Env = append(os.Environ(), g.Env()...)
		out, err := cmd.Output()
		cancel()
		if err != nil {
			c.auditRetry(ctx, j.id, j.attempts, fmt.Sprint(err))
			continue
		}
		var x auditOutput
		if json.Unmarshal(out, &x) != nil {
			c.auditRetry(ctx, j.id, j.attempts, "invalid audit output")
			continue
		}
		raw, _ := json.Marshal(x.Identity)
		cand, _ := json.Marshal(x.Identity.WebRTCCandidates)
		leak := false
		for _, v := range x.Identity.WebRTCCandidates {
			lc := strings.ToLower(v)
			if strings.Contains(lc, " typ host ") && !strings.Contains(lc, ".local ") {
				leak = true
			}
			if strings.Contains(lc, " typ srflx ") {
				leak = true
			}
		}
		tm := strings.EqualFold(x.Identity.Timezone, g.Timezone)
		lm := strings.EqualFold(x.Identity.Language, g.Locale)
		geo := "match"
		if !tm || !lm {
			geo = "mismatch"
		}
		ns := "browser:" + strings.ReplaceAll(j.id, "-", "") + ":" + j.rt
		_, err = c.DB.ExecContext(ctx, `INSERT INTO account_browser_identities(account_id,runtime,profile_namespace,platform,user_agent,timezone,language,screen,hardware_concurrency,device_memory,touch_support,canvas_hash,webgl_vendor,webgl_renderer,webgl_hash,audio_hash,client_rects_hash,fonts_hash,webrtc_candidates,webrtc_leak,raw,checked_at,audit_status,runtime_version,runtime_engine,expected_country,expected_timezone,expected_locale,timezone_match,locale_match,geo_consistency) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,now(),'pass',$22,$23,$24,$25,$26,$27,$28,$29) ON CONFLICT(account_id,runtime) DO UPDATE SET profile_namespace=EXCLUDED.profile_namespace,platform=EXCLUDED.platform,user_agent=EXCLUDED.user_agent,timezone=EXCLUDED.timezone,language=EXCLUDED.language,screen=EXCLUDED.screen,hardware_concurrency=EXCLUDED.hardware_concurrency,device_memory=EXCLUDED.device_memory,touch_support=EXCLUDED.touch_support,canvas_hash=EXCLUDED.canvas_hash,webgl_vendor=EXCLUDED.webgl_vendor,webgl_renderer=EXCLUDED.webgl_renderer,webgl_hash=EXCLUDED.webgl_hash,audio_hash=EXCLUDED.audio_hash,client_rects_hash=EXCLUDED.client_rects_hash,fonts_hash=EXCLUDED.fonts_hash,webrtc_candidates=EXCLUDED.webrtc_candidates,webrtc_leak=EXCLUDED.webrtc_leak,raw=EXCLUDED.raw,checked_at=now(),audit_status='pass',audit_error=NULL,runtime_version=EXCLUDED.runtime_version,runtime_engine=EXCLUDED.runtime_engine,expected_country=EXCLUDED.expected_country,expected_timezone=EXCLUDED.expected_timezone,expected_locale=EXCLUDED.expected_locale,timezone_match=EXCLUDED.timezone_match,locale_match=EXCLUDED.locale_match,geo_consistency=EXCLUDED.geo_consistency`, j.id, j.rt, ns, x.Identity.Platform, x.Identity.UserAgent, x.Identity.Timezone, x.Identity.Language, x.Identity.Screen, x.Identity.HardwareConcurrency, x.Identity.DeviceMemory, x.Identity.TouchSupport, x.Identity.CanvasHash, x.Identity.WebGLVendor, x.Identity.WebGLRenderer, x.Identity.WebGLHash, x.Identity.AudioHash, x.Identity.ClientRectsHash, x.Identity.FontsHash, cand, leak, raw, x.RuntimeVersion, x.RuntimeEngine, g.Country, g.Timezone, g.Locale, tm, lm, geo)
		if err != nil {
			c.auditRetry(ctx, j.id, j.attempts, err.Error())
			continue
		}
		_, _ = c.DB.ExecContext(ctx, `DELETE FROM browser_audit_queue WHERE account_id=$1`, j.id)
	}
}
func (c Container) auditRetry(ctx context.Context, id string, attempts int, detail string) {
	n := attempts + 1
	delay := 15 * time.Second * time.Duration(1<<min(n-1, 5))
	_, _ = c.DB.ExecContext(ctx, `UPDATE browser_audit_queue SET attempts=$2,last_error=$3,not_before=now()+($4::text||' seconds')::interval WHERE account_id=$1`, id, n, detail, int(delay.Seconds()))
}
