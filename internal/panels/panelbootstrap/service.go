package panelbootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/inventory"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/reality"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/reality/credentials"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/runtimecap"
)

var ErrRealityWarming = errors.New("reality stability warming")

type Secrets interface {
	Get(context.Context, string, string) ([]byte, error)
	Put(context.Context, string, string, string, []byte) error
}

type Service struct {
	DB         *sql.DB
	Secrets    Secrets
	SSH        provisioning.SSHClient
	ManagedKey string
}

func (s Service) BootstrapPanel(ctx context.Context, p readyworker.Panel) error {
	if s.DB == nil || s.Secrets == nil || p.ID == "" || s.ManagedKey == "" {
		return errors.New("bootstrap config")
	}
	var acc, did, host, user, keyref, puser, pref, path, state string
	var pport int
	err := s.DB.QueryRowContext(ctx, `
SELECT pi.account_id::text,pi.droplet_id::text,d.host,
d.profile_snapshot->>'ssh_user',d.profile_snapshot->>'ssh_key_secret_ref',
x.username,x.password_secret_ref,x.port,x.web_path,r.state
FROM panel_instances pi
JOIN deployments d ON d.droplet_id=pi.droplet_id
JOIN droplets r ON r.id=pi.droplet_id
JOIN xui_panel_deployments x ON x.droplet_id=pi.droplet_id AND x.generation=d.postinstall_generation
WHERE pi.id=$1 AND pi.enabled=true
`, p.ID).Scan(&acc, &did, &host, &user, &keyref, &puser, &pref, &pport, &path, &state)
	if err != nil {
		return err
	}
	if state != "READY" && state != "EXPIRING" && state != "RETIRING" {
		return errors.New("panel not operational")
	}
	target := provisioning.Target{AccountID: acc, DropletID: did, Host: host, User: user, KeySecretRef: keyref}
	exec := sanaei.SSHSessionExecutor{SSH: s.SSH, Target: target, PrivateKeySecretRef: keyref, PanelPasswordSecretRef: pref, AccountID: acc, Username: puser, Port: pport, BasePath: path, DialHost: "127.0.0.1", Secrets: s.Secrets}
	disc, err := sanaei.DiscoverWithExecutor(ctx, exec, "")
	if err != nil {
		return err
	}
	if err = (panels.SQLStore{DB: s.DB}).SaveDiscovery(ctx, p.ID, disc); err != nil {
		return err
	}
	snap, err := sanaei.ReadInventory(ctx, exec, p.ID)
	if err != nil {
		return err
	}
	if _, err = (inventory.SQLStore{DB: s.DB}).Sync(ctx, snap); err != nil {
		return err
	}
	key, err := s.Secrets.Get(ctx, acc, keyref)
	if err != nil {
		return err
	}
	defer credentials.Wipe(key)
	run := func(ctx context.Context, cmd string) (string, error) { return s.SSH.Run(ctx, target, key, cmd) }
	var mode string
	var manualRaw []byte
	if err = s.DB.QueryRowContext(ctx, "SELECT sni_selection_mode,manual_snis FROM global_config_policies WHERE policy_key='reality'").Scan(&mode, &manualRaw); err != nil {
		return err
	}
	targets := ""
	if mode == "manual" {
		var names []string
		if json.Unmarshal(manualRaw, &names) != nil || len(names) == 0 {
			return errors.New("manual reality SNI policy empty")
		}
		clean := names[:0]
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name == "" || strings.ContainsAny(name, "/\\ \t\r\n") {
				return errors.New("invalid manual reality SNI")
			}
			clean = append(clean, name)
		}
		targets = strings.Join(clean, ",")
	} else if mode != "scored" {
		return errors.New("invalid reality SNI selection mode")
	}
	scanned, err := sanaei.ScanRealityTargets(ctx, exec, targets)
	if err != nil {
		return err
	}
	store := reality.SQLStore{DB: s.DB}
	now := time.Now().UTC()
	byTarget := map[string]sanaei.RealityScanResult{}
	for _, x := range scanned {
		if !x.Feasible || x.Host == "" || x.Port < 1 {
			continue
		}
		byTarget[x.Host] = x
		o := reality.Observation{
			Candidate: reality.Candidate{Target: x.Host, ServerName: x.Host, Port: x.Port},
			Reachable: true, TLSVersion: x.TLSVersion, CertValid: x.CertValid,
			HTTP2: x.ALPN == "h2", Samples: 1, Successes: 1,
			LatencyMS: []int64{x.LatencyMS}, ObservedAt: now,
		}
		if err = store.Save(ctx, p.ID, o); err != nil {
			return err
		}
	}
	history, err := store.Recent(ctx, p.ID, 5)
	if err != nil {
		return err
	}
	stable := reality.Aggregate(history, reality.StabilityPolicy{MinObservations: 3, MinEligibleRatio: .8, SwitchMargin: 5})
	choice, _, err := reality.ChooseStable(nil, stable, reality.StabilityPolicy{MinObservations: 3, MinEligibleRatio: .8, SwitchMargin: 5})
	if errors.Is(err, reality.ErrInsufficientHistory) {
		return ErrRealityWarming
	}
	if err != nil {
		return err
	}
	selected, ok := byTarget[choice.Candidate.Target]
	if !ok {
		return ErrRealityWarming
	}
	full, err := sanaei.ScanRealityTargets(ctx, exec, selected.Target)
	if err != nil || len(full) != 1 || !full[0].Feasible || len(full[0].ServerNames) == 0 {
		return ErrRealityWarming
	}
	selected = full[0]
	sni := selected.ServerNames[0]
	for _, name := range selected.ServerNames {
		if name == selected.Host {
			sni = selected.Host
			break
		}
	}
	choice.Candidate.ServerName = sni
	score := reality.Score{Candidate: choice.Candidate, Eligible: true, MedianLatencyMS: choice.MedianLatencyMS, SuccessRatio: choice.SuccessRatio, Value: choice.Score, Reason: "sanaei-find-targets-use"}
	if err = store.Select(ctx, p.ID, score, now); err != nil {
		return err
	}
	if err = store.SaveScanSelection(ctx, reality.ScanSelection{PanelID: p.ID, ServerNames: selected.ServerNames, TLSVersion: selected.TLSVersion, ALPN: selected.ALPN, CurveID: selected.CurveID, CertValid: selected.CertValid, CertChainValid: selected.CertChainValid, LatencyMS: selected.LatencyMS, ScannedAt: now}); err != nil {
		return err
	}
	xrayPath, err := (runtimecap.XrayResolver{Run: run}).Resolve(ctx)
	if err != nil {
		return err
	}
	_, err = (credentials.Registry{DB: s.DB, Secrets: s.Secrets, Keys: credentials.XrayGenerator{Run: run, Binary: xrayPath}}).Ensure(ctx, acc, p.ID, s.ManagedKey)
	return err
}
