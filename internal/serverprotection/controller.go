package serverprotection

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/reality/credentials"
)

type Secrets interface {
	Get(context.Context, string, string) ([]byte, error)
}

// GuardianSSH is the existing pinned SSH transport contract, also usable by isolated fault tests.
type GuardianSSH interface {
	Run(context.Context, provisioning.Target, []byte, string) (string, error)
	Upload(context.Context, provisioning.Target, []byte, string, string, os.FileMode) error
}
type Controller struct {
	Admit       func(context.Context, func(context.Context) error) error
	DB          *sql.DB
	Secrets     Secrets
	SSH         GuardianSSH
	ArtifactDir string
	// Nil upgrades all nodes. A non-nil allowlist stages binary upgrades only;
	// existing agents still receive enable/disable policy and status polling.
	UpgradePanels map[string]bool
}

func ParseUpgradePanels(raw string) map[string]bool {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	panels := map[string]bool{}
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" && p != "none" {
			panels[p] = true
		}
	}
	return panels
}

func (c Controller) upgradeAllowed(panel string) bool {
	return c.UpgradePanels == nil || c.UpgradePanels[panel]
}

type nodeTarget struct {
	Panel, Account, Droplet, Host, User, KeyRef, URL string
	Policy                                           Policy
	RequireDue                                       bool
}

// Run observes enabled nodes; cleanup has its own independently supervised loop.
func (c Controller) Run(ctx context.Context)        { c.runLane(ctx, true, 6) }
func (c Controller) RunCleanup(ctx context.Context) { c.runLane(ctx, false, 2) }

// Rolling discovery never waits for the slowest peer before renewing healthy
// receipts. Only lane-sized work is admitted; no SQL lease is held while waiting
// for the shared parent budget. The local set prevents duplicate queued work;
// PostgreSQL advisory locks and revision fences remain authoritative across owners.
func (c Controller) runLane(parent context.Context, enabled bool, concurrency int) {
	ctx, cancel := context.WithCancel(parent)
	var wg sync.WaitGroup
	defer func() { cancel(); wg.Wait() }()
	completed := make(chan string, concurrency)
	inFlight := make(map[string]bool)
	retryAfter := make(map[string]time.Time)
	var discoveryRetry time.Time
	timer := time.NewTimer(0)
	defer timer.Stop()
	reset := func(delay time.Duration) {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(delay)
	}
	var lastSync time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case panel := <-completed:
			delete(inFlight, panel)
			// A competing owner or unavailable database may leave next_check_at due.
			// Keep retries bounded without inventing or advancing a receipt.
			retryAfter[panel] = time.Now().Add(time.Second)
			reset(0)
		case <-timer.C:
			if ctx.Err() != nil {
				return
			}
			supervision.Pulse(ctx)
			if delay := time.Until(discoveryRetry); delay > 0 {
				reset(delay)
				continue
			}
			if len(inFlight) >= concurrency {
				reset(time.Second)
				continue
			}
			var err error
			var targets []nodeTarget
			discover := func(work context.Context) error {
				queryCtx, done := context.WithTimeout(work, 5*time.Second)
				defer done()
				if c.DB == nil || c.Secrets == nil {
					err = fmt.Errorf("protection controller not configured")
				} else if lastSync.IsZero() || time.Since(lastSync) >= 5*time.Second {
					err = (Store{DB: c.DB}).Sync(queryCtx)
					if err == nil {
						lastSync = time.Now()
					}
				}
				if err == nil {
					excluded := make([]string, 0, len(inFlight)+len(retryAfter))
					for panel, until := range retryAfter {
						if time.Now().Before(until) {
							excluded = append(excluded, panel)
						} else {
							delete(retryAfter, panel)
						}
					}
					for panel := range inFlight {
						excluded = append(excluded, panel)
					}
					targets, err = c.selectTargets(queryCtx, &enabled, concurrency-len(inFlight), excluded)
				}
				return err
			}
			if c.Admit != nil {
				err = c.Admit(ctx, discover)
			} else {
				err = discover(ctx)
			}
			if err != nil {
				if ctx.Err() == nil {
					log.Printf("server protection discovery (enabled=%t): %v", enabled, err)
				}
				discoveryRetry = time.Now().Add(5 * time.Second)
				reset(5 * time.Second)
				continue
			}
			for _, target := range targets {
				if ctx.Err() != nil {
					break
				}
				target.RequireDue = true
				inFlight[target.Panel] = true
				wg.Add(1)
				go func(t nodeTarget) {
					defer wg.Done()
					if c.Admit != nil {
						_ = c.Admit(ctx, func(work context.Context) error { c.reconcile(work, t); return work.Err() })
					} else {
						c.reconcile(ctx, t)
					}
					select {
					case completed <- t.Panel:
					case <-ctx.Done():
					}
				}(target)
			}
			if len(inFlight) == 0 {
				supervision.Idle(ctx)
			}
			reset(time.Second)
		}
	}
}

// Round retains combined one-shot compatibility; production registers only the two lane loops.
func (c Controller) Round(ctx context.Context) error { return c.round(ctx, nil, 8) }
func (c Controller) round(ctx context.Context, enabled *bool, concurrency int) error {
	if c.DB == nil || c.Secrets == nil {
		return fmt.Errorf("protection controller not configured")
	}
	if e := (Store{DB: c.DB}).Sync(ctx); e != nil {
		return e
	}
	targets, e := c.selectTargets(ctx, enabled, 64, nil)
	if e != nil {
		return e
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, t := range targets {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		}
		wg.Add(1)
		go func(t nodeTarget) {
			defer wg.Done()
			defer func() { <-sem }()
			if c.Admit != nil {
				_ = c.Admit(ctx, func(ctx context.Context) error { c.reconcile(ctx, t); return ctx.Err() })
			} else {
				c.reconcile(ctx, t)
			}
		}(t)
	}
	wg.Wait()
	return ctx.Err()
}
func (c Controller) selectTargets(ctx context.Context, enabled *bool, limit int, excluded []string) ([]nodeTarget, error) {
	if excluded == nil {
		excluded = []string{}
	}
	rows, e := c.DB.QueryContext(ctx, `SELECT n.panel_id::text,p.account_id::text,p.droplet_id::text,COALESCE(dp.host,''),
 COALESCE(NULLIF(dp.profile_snapshot->>'ssh_user',''),'root'),COALESCE(dp.profile_snapshot->>'ssh_key_secret_ref',''),p.base_url,
 n.desired_revision,n.desired_enabled
 FROM server_protection_nodes n JOIN panel_instances p ON p.id=n.panel_id
 JOIN droplets dr ON dr.id=p.droplet_id JOIN deployments dp ON dp.droplet_id=dr.id
 WHERE dr.state<>'DELETED' AND n.next_check_at<=now() AND ($1::boolean IS NULL OR n.desired_enabled=$1) AND NOT(n.panel_id=ANY($2::uuid[]))
 ORDER BY n.next_check_at,n.panel_id LIMIT $3`, enabled, pq.Array(excluded), limit)
	if e != nil {
		return nil, e
	}
	var targets []nodeTarget
	for rows.Next() {
		var t nodeTarget
		if e = rows.Scan(&t.Panel, &t.Account, &t.Droplet, &t.Host, &t.User, &t.KeyRef, &t.URL, &t.Policy.Revision, &t.Policy.Enabled); e != nil {
			rows.Close()
			return nil, e
		}
		t.Policy.PanelID = t.Panel
		t.Policy.ExcludedPorts = []int{22}
		u, err := url.Parse(t.URL)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("invalid panel URL")
		}
		port := u.Port()
		if port == "" {
			if u.Scheme == "https" {
				port = "443"
			} else {
				port = "80"
			}
		}
		n, err := strconv.Atoi(port)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if n != 22 {
			t.Policy.ExcludedPorts = append(t.Policy.ExcludedPorts, n)
		}
		targets = append(targets, t)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	return targets, nil
}

func (c Controller) reconcile(parent context.Context, t nodeTarget) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	conn, e := c.DB.Conn(ctx)
	if e != nil {
		return
	}
	defer conn.Close()
	key := "server-protection:" + t.Panel
	var acquired bool
	if e = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,0))", key).Scan(&acquired); e != nil || !acquired {
		return
	}
	defer func() {
		release, done := context.WithTimeout(context.Background(), 2*time.Second)
		defer done()
		if _, err := conn.ExecContext(release, "SELECT pg_advisory_unlock(hashtextextended($1,0))", key); err != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	// Reject superseded work before opening a network connection.
	result, e := conn.ExecContext(ctx, "UPDATE server_protection_nodes SET next_check_at=now()+interval '35 seconds' WHERE panel_id=$1 AND desired_revision=$2 AND desired_enabled=$3 AND (NOT $4 OR next_check_at<=now())", t.Panel, t.Policy.Revision, t.Policy.Enabled, t.RequireDue)
	if e != nil {
		return
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return
	}
	var st Status
	if c.SSH == nil {
		e = fmt.Errorf("guardian SSH transport not configured")
	} else if t.Host == "" || t.KeyRef == "" {
		e = fmt.Errorf("missing managed SSH target")
	} else {
		var private []byte
		private, e = c.Secrets.Get(ctx, t.Account, t.KeyRef)
		if e == nil {
			defer credentials.Wipe(private)
			st, e = c.apply(ctx, t, private)
		}
	}
	// A planned worker shutdown is not evidence that the remote guardian failed.
	if parent.Err() != nil {
		return
	}
	receiptCtx, done := context.WithTimeout(parent, 3*time.Second)
	defer done()
	// Publish on the session owning the advisory lock. A dead owner must never
	// borrow a replacement session and overwrite its successor observation.
	if err := receipt(receiptCtx, conn, t.Panel, t.Policy, st, e); err != nil && err != ErrConflict {
		log.Printf("server protection receipt: %v", err)
	}
}
func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
func (c Controller) remote(ctx context.Context, t nodeTarget, key []byte, command string) (string, error) {
	if t.User != "root" {
		command = "sudo -n sh -c " + quote(command)
	}
	return c.SSH.Run(ctx, provisioning.Target{AccountID: t.Account, DropletID: t.Droplet, Host: t.Host, User: t.User, KeySecretRef: t.KeyRef}, key, command)
}
func (c Controller) apply(ctx context.Context, t nodeTarget, key []byte) (Status, error) {
	var st Status
	probeCtx, stopProbe := context.WithTimeout(ctx, 6*time.Second)
	out, e := c.remote(probeCtx, t, key, guardianInventoryCommand)
	stopProbe()
	if e != nil {
		return st, e
	}
	fields := strings.SplitN(strings.TrimSpace(out), "\n", 7)
	if len(fields) != 2 && len(fields) != 7 {
		return st, fmt.Errorf("invalid guardian inventory")
	}
	arch := ""
	switch fields[0] {
	case "x86_64":
		arch = "amd64"
	case "aarch64", "arm64":
		arch = "arm64"
	default:
		return st, fmt.Errorf("unsupported architecture")
	}
	// Turning protection off must not wait for a binary transfer on a pressured
	// host. The compatible owned installation can apply the revision fence,
	// stop its service and remove its rules using the existing cleanup path.
	// Fresh receipt validation remains authoritative; enabled nodes still
	// require the desired artifact and executing-image proof.
	if compatibleDisabledCleanup(fields, t.Policy) {
		out, err := c.remote(ctx, t, key, installCommand(t.Policy, "", ""))
		if err != nil {
			return st, fmt.Errorf("guardian cleanup: %w: %.300s", err, strings.TrimSpace(out))
		}
		if err = json.Unmarshal([]byte(strings.TrimSpace(out)), &st); err != nil {
			return st, fmt.Errorf("invalid guardian cleanup receipt")
		}
		return st, st.ValidateReceipt(t.Policy)
	}
	dir := c.ArtifactDir
	if dir == "" {
		dir = "/opt/digital-ocean-bot/bin"
	}
	artifact := filepath.Join(dir, "server-guardian-linux-"+arch)
	b, e := os.ReadFile(artifact)
	if e != nil {
		return st, fmt.Errorf("guardian artifact unavailable")
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(b))
	b = nil
	// A matching owned installation needs observation only. Do not repeatedly
	// configure files or call systemctl start on every telemetry poll. Genuine
	// resource pressure/unsupported ports are reported, never "repaired" by restart.
	if t.Policy.Enabled && fields[1] == hash && len(fields) == 7 {
		if observed, ok := verifiedInventory(fields, t.Policy, hash); ok {
			return observed, nil
		}
		// A fresh/live cached sample without executing-image proof must not trigger
		// generic installation. Preserve the running protector and report uncertainty.
		decodeErr := json.Unmarshal([]byte(fields[6]), &st)
		pid, pidErr := strconv.Atoi(fields[5])
		if pidErr != nil || pid < 0 || (pid == 0 && st.AgentRunning) ||
			(pid > 0 && (decodeErr != nil || !st.AgentRunning || st.AgentPID != pid || st.AgentStartTicks == "" || st.AgentBinarySHA256 != hash)) {
			return st, fmt.Errorf("running guardian artifact or sample producer is unproven; observation required")
		}
	}

	stage := ""
	if fields[1] == "missing" && !c.upgradeAllowed(t.Panel) {
		return st, fmt.Errorf("guardian initial installation pending staged rollout")
	}
	if fields[1] != hash && c.upgradeAllowed(t.Panel) {
		// mktemp creates an owned non-symlink staging file; SCP never writes a fixed public path.
		raw, err := c.SSH.Run(ctx, provisioning.Target{AccountID: t.Account, DropletID: t.Droplet, Host: t.Host, User: t.User, KeySecretRef: t.KeyRef}, key, "umask 077; mktemp /var/tmp/dob-guardian.XXXXXXXXXXXX")
		if err != nil {
			return st, err
		}
		stage = strings.TrimSpace(raw)
		if !strings.HasPrefix(stage, "/var/tmp/dob-guardian.") || strings.ContainsAny(stage, " \n\r'") {
			return st, fmt.Errorf("invalid guardian staging path")
		}
		defer func() {
			cleanupCtx, done := context.WithTimeout(context.Background(), 4*time.Second)
			defer done()
			_, _ = c.SSH.Run(cleanupCtx, provisioning.Target{AccountID: t.Account, DropletID: t.Droplet, Host: t.Host, User: t.User, KeySecretRef: t.KeyRef}, key, "rm -f -- "+quote(stage))
		}()
		target := provisioning.Target{AccountID: t.Account, DropletID: t.Droplet, Host: t.Host, User: t.User, KeySecretRef: t.KeyRef}
		if err = c.SSH.Upload(ctx, target, key, artifact, stage, 0600); err != nil {
			return st, err
		}
	}
	out, e = c.remote(ctx, t, key, installCommand(t.Policy, stage, hash))
	if e != nil {
		return st, fmt.Errorf("guardian installation: %w: %.300s", e, strings.TrimSpace(out))
	}
	if e = json.Unmarshal([]byte(strings.TrimSpace(out)), &st); e != nil {
		return st, fmt.Errorf("invalid guardian status receipt")
	}
	if t.Policy.Enabled && c.upgradeAllowed(t.Panel) {
		verifyCtx, finish := context.WithTimeout(ctx, 6*time.Second)
		observation, err := c.remote(verifyCtx, t, key, guardianInventoryCommand)
		finish()
		if err != nil {
			return st, fmt.Errorf("guardian activation observation: %w", err)
		}
		fields := strings.SplitN(strings.TrimSpace(observation), "\n", 7)
		verified, ok := verifiedInventory(fields, t.Policy, hash)
		if !ok {
			return st, fmt.Errorf("guardian activation lacks executing artifact proof")
		}
		return verified, nil
	}
	return st, nil
}

// Inventory only reports the unit hash/status for an owned installation.
// installCommand rechecks ownership and unsafe paths before any mutation.
func compatibleDisabledCleanup(fields []string, p Policy) bool {
	if p.Enabled || len(fields) != 7 || len(fields[1]) != sha256.Size*2 {
		return false
	}
	if _, err := hex.DecodeString(fields[1]); err != nil {
		return false
	}
	if fields[3] != fmt.Sprintf("%x", sha256.Sum256([]byte(Unit))) {
		return false
	}
	var st Status
	return json.Unmarshal([]byte(fields[6]), &st) == nil && st.Version == Version
}

const guardianInventoryCommand = `set -eu
uname -m
if [ -f /usr/local/libexec/dob-server-guardian ]; then sha256sum /usr/local/libexec/dob-server-guardian | cut -d' ' -f1; else printf 'missing\n'; fi
for path in /etc/dob-server-guardian/policy.json /etc/systemd/system/dob-server-guardian.service; do
 if [ -f "$path" ] && [ ! -L "$path" ] && grep -Fxq 'digital-ocean-bot server-protection v1' /etc/dob-server-guardian/owner 2>/dev/null; then sha256sum "$path" | cut -d' ' -f1; else printf 'missing\n'; fi
done
unit_state=$(systemctl is-enabled dob-server-guardian.service 2>/dev/null) || true
printf '%s\n' "${unit_state:-unknown}"
identity_before=$(systemctl show dob-server-guardian.service -p MainPID -p InvocationID 2>/dev/null) || identity_before=''
sample='{}'
if [ -x /usr/local/libexec/dob-server-guardian ] && [ ! -L /usr/local/libexec/dob-server-guardian ] && grep -Fxq 'digital-ocean-bot server-protection v1' /etc/dob-server-guardian/owner 2>/dev/null; then sample=$(/usr/local/libexec/dob-server-guardian status 2>/dev/null) || sample='{}'; fi
identity_after=$(systemctl show dob-server-guardian.service -p MainPID -p InvocationID 2>/dev/null) || identity_after=''
pid=unknown
if [ -n "$identity_before" ] && [ "$identity_before" = "$identity_after" ]; then pid=$(printf '%s\n' "$identity_after" | sed -n 's/^MainPID=//p'); fi
printf '%s\n%s\n' "${pid:-unknown}" "$sample"
`

func verifiedInventory(fields []string, p Policy, hash string) (Status, bool) {
	var st Status
	if len(fields) != 7 || fields[1] != hash {
		return st, false
	}
	raw, _ := json.Marshal(p)
	policyHash := fmt.Sprintf("%x", sha256.Sum256(append(raw, '\n')))
	unitHash := fmt.Sprintf("%x", sha256.Sum256([]byte(Unit)))
	if fields[2] != policyHash || fields[3] != unitHash || fields[4] != "enabled" || json.Unmarshal([]byte(fields[6]), &st) != nil {
		return st, false
	}
	return st, st.CurrentAgent(p) && st.AgentPID > 0 && strconv.Itoa(st.AgentPID) == fields[5] && st.AgentStartTicks != "" && st.AgentBinarySHA256 == hash
}

const Unit = `# Managed by digital-ocean-bot server-protection v1
[Unit]
Description=Digital Ocean Bot local server protection
After=network.target
StartLimitIntervalSec=60
StartLimitBurst=5

[Service]
Type=simple
ExecStart=/usr/local/libexec/dob-server-guardian run
Restart=on-failure
RestartSec=2
TimeoutStopSec=5
RuntimeDirectory=dob-server-guardian
RuntimeDirectoryMode=0700
RuntimeDirectoryPreserve=yes
StateDirectory=dob-server-guardian
StateDirectoryMode=0700
UMask=0077
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/dob-server-guardian /run/dob-server-guardian /run/lock
PrivateTmp=true
MemoryHigh=64M
MemoryMax=128M
TasksMax=128
CPUWeight=1000

[Install]
WantedBy=multi-user.target
`

func installCommand(p Policy, stage, hash string) string {
	raw, _ := json.Marshal(p)
	script := `set -eu
umask 077
mkdir -p /etc/dob-server-guardian /var/lib/dob-server-guardian /run/dob-server-guardian /usr/local/libexec
exec 9>/run/lock/dob-server-guardian-install.lock
flock -w 5 9
unit_changed=0
unit=/etc/systemd/system/dob-server-guardian.service
for path in "$unit" "$unit.new" /etc/dob-server-guardian /var/lib/dob-server-guardian /run/dob-server-guardian /usr/local/libexec/dob-server-guardian /usr/local/libexec/dob-server-guardian.new; do
 if [ -L "$path" ]; then printf 'unsafe guardian symlink\n' >&2; exit 1; fi
done
marker=/etc/dob-server-guardian/owner
if [ -f "$marker" ]; then
 grep -Fxq 'digital-ocean-bot server-protection v1' "$marker" || exit 1
else
 if [ -e /usr/local/libexec/dob-server-guardian ] || [ -e "$unit" ]; then printf 'foreign guardian installation\n' >&2; exit 1; fi
 printf 'digital-ocean-bot server-protection v1\n' > "$marker"
fi
if [ -e "$unit" ] && ! head -n 1 "$unit" | grep -Fxq '# Managed by digital-ocean-bot server-protection v1'; then printf 'foreign guardian unit\n' >&2; exit 1; fi
`
	// Fence a delayed installer before replacing an existing binary or unit.
	script += "if [ -x " + BinaryPath + " ]; then printf %s " + quote(base64.StdEncoding.EncodeToString(raw)) + " | base64 -d | " + BinaryPath + " configure; fi\n"
	if stage != "" {
		script += "stage=" + quote(stage) + "\ntrap 'rm -f -- \"$stage\"' EXIT\nprintf '%s  %s\\n' " + quote(hash) + " \"$stage\" | sha256sum -c - >/dev/null\ninstall -m 0755 \"$stage\" /usr/local/libexec/dob-server-guardian.new\nmv /usr/local/libexec/dob-server-guardian.new /usr/local/libexec/dob-server-guardian\n"
	}
	script += guardianUnitCommand()
	script += "printf %s " + quote(base64.StdEncoding.EncodeToString(raw)) + " | base64 -d | " + BinaryPath + " configure\n"
	if p.Enabled {
		if stage != "" {
			script += "unit_changed=1\n"
		}
		script += guardianServiceCommand(true) + BinaryPath + " wait-status " + strconv.FormatInt(p.Revision, 10) + "\n"
	} else {
		script += guardianServiceCommand(false) + BinaryPath + " cleanup\n" + BinaryPath + " status\n"
	}
	return script
}

func guardianUnitCommand() string {
	return "printf %s " + quote(base64.StdEncoding.EncodeToString([]byte(Unit))) + " | base64 -d > \"$unit.new\"\nif ! cmp -s \"$unit.new\" \"$unit\"; then mv \"$unit.new\" \"$unit\"; systemctl daemon-reload; unit_changed=1; else rm -f \"$unit.new\"; fi\n"
}

// Enabling an already enabled unit still makes systemctl reload the manager.
// Only perform that operation when needed, retaining start/stop and fresh receipt.
func guardianServiceCommand(enabled bool) string {
	script := "state_rc=0\nunit_state=$(systemctl is-enabled " + UnitName + " 2>/dev/null) || state_rc=$?\n"
	if enabled {
		script += "if [ \"$unit_changed\" = 1 ]; then systemctl restart " + UnitName + " >/dev/null 2>&1; fi\n"
		script += "if [ \"$unit_state\" != enabled ] || [ \"$state_rc\" != 0 ]; then systemctl enable " + UnitName + " >/dev/null 2>&1; fi\n"
		return script + "systemctl start " + UnitName + " >/dev/null 2>&1\n"
	}
	script += "if [ \"$unit_state\" != disabled ] || [ \"$state_rc\" != 1 ]; then systemctl disable " + UnitName + " >/dev/null 2>&1; fi\n"
	return script + "systemctl stop " + UnitName + " >/dev/null 2>&1\n"
}
