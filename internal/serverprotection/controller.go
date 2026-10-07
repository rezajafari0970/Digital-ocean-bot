package serverprotection

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
type Controller struct {
	Admit       func(context.Context, func(context.Context) error) error
	DB          *sql.DB
	Secrets     Secrets
	SSH         provisioning.SSHClient
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
}

func (c Controller) Run(ctx context.Context) {
	for {
		rctx, cancel := context.WithTimeout(ctx, 90*time.Second)
		if e := c.Round(rctx); e != nil && ctx.Err() == nil {
			log.Printf("server protection reconciliation: %v", e)
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}
func (c Controller) Round(ctx context.Context) error {
	if c.DB == nil || c.Secrets == nil {
		return fmt.Errorf("protection controller not configured")
	}
	if e := (Store{DB: c.DB}).Sync(ctx); e != nil {
		return e
	}
	rows, e := c.DB.QueryContext(ctx, `SELECT n.panel_id::text,p.account_id::text,p.droplet_id::text,COALESCE(dp.host,''),
 COALESCE(NULLIF(dp.profile_snapshot->>'ssh_user',''),'root'),COALESCE(dp.profile_snapshot->>'ssh_key_secret_ref',''),p.base_url,
 n.desired_revision,n.desired_enabled
 FROM server_protection_nodes n JOIN panel_instances p ON p.id=n.panel_id
 JOIN droplets dr ON dr.id=p.droplet_id JOIN deployments dp ON dp.droplet_id=dr.id
 WHERE dr.state<>'DELETED' AND n.next_check_at<=now()
 ORDER BY n.desired_enabled,n.next_check_at,n.panel_id LIMIT 64`)
	if e != nil {
		return e
	}
	var targets []nodeTarget
	for rows.Next() {
		var t nodeTarget
		if e = rows.Scan(&t.Panel, &t.Account, &t.Droplet, &t.Host, &t.User, &t.KeyRef, &t.URL, &t.Policy.Revision, &t.Policy.Enabled); e != nil {
			rows.Close()
			return e
		}
		t.Policy.PanelID = t.Panel
		t.Policy.ExcludedPorts = []int{22}
		u, err := url.Parse(t.URL)
		if err != nil {
			rows.Close()
			return fmt.Errorf("invalid panel URL")
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
			return err
		}
		if n != 22 {
			t.Policy.ExcludedPorts = append(t.Policy.ExcludedPorts, n)
		}
		targets = append(targets, t)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	sem := make(chan struct{}, 8)
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
func (c Controller) reconcile(ctx context.Context, t nodeTarget) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
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
	result, e := conn.ExecContext(ctx, "UPDATE server_protection_nodes SET next_check_at=now()+interval '35 seconds' WHERE panel_id=$1 AND desired_revision=$2 AND desired_enabled=$3", t.Panel, t.Policy.Revision, t.Policy.Enabled)
	if e != nil {
		return
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return
	}
	var st Status
	if t.Host == "" || t.KeyRef == "" {
		e = fmt.Errorf("missing managed SSH target")
	} else {
		var private []byte
		private, e = c.Secrets.Get(ctx, t.Account, t.KeyRef)
		if e == nil {
			defer credentials.Wipe(private)
			st, e = c.apply(ctx, t, private)
		}
	}
	receiptCtx, done := context.WithTimeout(context.Background(), 3*time.Second)
	defer done()
	if err := (Store{DB: c.DB}).Receipt(receiptCtx, t.Panel, t.Policy, st, e); err != nil && err != ErrConflict {
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
	out, e := c.remote(ctx, t, key, `set -eu
uname -m
if [ -f /usr/local/libexec/dob-server-guardian ]; then sha256sum /usr/local/libexec/dob-server-guardian | cut -d' ' -f1; else printf 'missing\n'; fi`)
	if e != nil {
		return st, e
	}
	fields := strings.Fields(out)
	if len(fields) != 2 {
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
	return st, nil
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
	script += "printf %s " + quote(base64.StdEncoding.EncodeToString([]byte(Unit))) + " | base64 -d > \"$unit.new\"\nif ! cmp -s \"$unit.new\" \"$unit\"; then mv \"$unit.new\" \"$unit\"; systemctl daemon-reload; unit_changed=1; else rm -f \"$unit.new\"; fi\n"
	script += "printf %s " + quote(base64.StdEncoding.EncodeToString(raw)) + " | base64 -d | " + BinaryPath + " configure\n"
	if p.Enabled {
		if stage != "" {
			script += "unit_changed=1\n"
		}
		script += "if [ \"$unit_changed\" = 1 ]; then systemctl restart " + UnitName + " >/dev/null 2>&1; fi\n"
		script += "systemctl enable --now " + UnitName + " >/dev/null 2>&1\n" + BinaryPath + " wait-status " + strconv.FormatInt(p.Revision, 10) + "\n"
	} else {
		script += "systemctl disable --now " + UnitName + " >/dev/null 2>&1\n" + BinaryPath + " cleanup\n" + BinaryPath + " status\n"
	}
	return script
}
