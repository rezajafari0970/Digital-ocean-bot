package configplane

import (
	"context"
	"database/sql"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"sort"
	"time"
)

// Selection is private to this profile/server/revision. No global Reality policy
// is written, and a scan failure never changes a sibling profile's transport.
func (s Service) resolveSNI(ctx context.Context, p Profile, panel string) (configdriver.Spec, error) {
	spec := p.Spec
	var selected string
	e := s.DB.QueryRowContext(ctx, "SELECT sni FROM config_driver_sni_selections WHERE profile_id=$1 AND panel_id=$2 AND revision=$3 AND verified_at>now()-interval '6 hours'", p.ID, panel, p.Revision).Scan(&selected)
	if e == nil {
		spec.SNI = selected
		return spec, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return spec, e
	}
	rt, e := s.Runtimes.Acquire(ctx, panel)
	if e != nil {
		return spec, errors.New("profile_sni_scan_unavailable")
	}
	targets := spec.SNICandidates
	if spec.SNIMode != "auto" {
		targets = spec.SNI
	}
	bounded, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()
	results, e := sanaei.ScanRealityTargets(bounded, rt.Session.Exec, targets)
	if e != nil {
		return spec, errors.New("profile_sni_scan_failed")
	}
	// A repeatedly failing automatic target may be replaced by another verified
	// candidate. Manual settings are never silently changed.
	if spec.SNIMode == "auto" {
		var failed string
		err := s.DB.QueryRowContext(ctx, "SELECT COALESCE(applied_spec->>'sni','') FROM config_driver_listeners WHERE profile_id=$1 AND panel_id=$2 AND failures>=3 AND error_code='profile_runtime_traffic_unverified'", p.ID, panel).Scan(&failed)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return spec, err
		}
		alternatives := []sanaei.RealityScanResult{}
		for _, r := range results {
			if r.Host != failed {
				alternatives = append(alternatives, r)
			}
		}
		if _, err := selectSNITarget(spec, alternatives); err == nil {
			results = alternatives
		}
	}
	candidate, e := selectSNITarget(spec, results)
	if e != nil {
		return spec, e
	}
	spec.SNI = candidate.Host
	_, e = s.DB.ExecContext(ctx, "INSERT INTO config_driver_sni_selections(profile_id,panel_id,revision,sni,verified_at,latency_ms) VALUES($1,$2,$3,$4,now(),$5) ON CONFLICT(profile_id,panel_id) DO UPDATE SET revision=EXCLUDED.revision,sni=EXCLUDED.sni,verified_at=EXCLUDED.verified_at,latency_ms=EXCLUDED.latency_ms", p.ID, panel, p.Revision, spec.SNI, candidate.LatencyMS)
	return spec, e
}
func selectSNITarget(spec configdriver.Spec, results []sanaei.RealityScanResult) (sanaei.RealityScanResult, error) {
	results = append([]sanaei.RealityScanResult(nil), results...)
	sort.SliceStable(results, func(i, j int) bool { return results[i].LatencyMS < results[j].LatencyMS })
	for _, candidate := range results {
		if !candidate.Feasible || candidate.PrivateTarget || !candidate.CertValid || !candidate.CertChainValid || candidate.TLSVersion != "1.3" || candidate.ALPN != "h2" || candidate.Port != 443 || candidate.LatencyMS < 0 {
			continue
		}
		if spec.SNIMode != "auto" && candidate.Host != spec.SNI {
			continue
		}
		probe := spec
		probe.SNI = candidate.Host
		if configdriver.Builtins().Validate(probe, true) != nil {
			continue
		}
		return candidate, nil
	}
	return sanaei.RealityScanResult{}, errors.New("profile_sni_no_verified_target")
}
