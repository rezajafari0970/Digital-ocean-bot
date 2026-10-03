package serverguardian

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/reality/credentials"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Secrets interface {
	Get(context.Context, string, string) ([]byte, error)
}
type Service struct {
	DB      *sql.DB
	Secrets Secrets
	SSH     provisioning.SSHClient
	Repair  bool
}
type target struct{ panel, account, droplet, host, user, keyRef string }
type Snapshot struct {
	XUIActive, Restarted, LockBusy, Reboot, DBPresent bool
	MemMB, DiskMB                                     int64
	Load                                              float64
	DBCheck, LastError                                string
}

func (s Service) Run(ctx context.Context) error {
	if s.DB == nil || s.Secrets == nil {
		return fmt.Errorf("server guardian config")
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT pi.id::text,pi.account_id::text,pi.droplet_id::text,d.host,COALESCE(d.profile_snapshot->>'ssh_user','root'),COALESCE(d.profile_snapshot->>'ssh_key_secret_ref','') FROM panel_instances pi JOIN droplets dr ON dr.id=pi.droplet_id JOIN accounts a ON a.id=dr.account_id JOIN deployments d ON d.droplet_id=dr.id WHERE pi.enabled=true AND a.provider_state='ACTIVE' AND dr.state IN ('READY','EXPIRING') AND (dr.expires_at IS NULL OR dr.expires_at>now()+interval '10 seconds') AND d.state='PANEL_COMPLETE' ORDER BY pi.id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ts []target
	for rows.Next() {
		var t target
		if err = rows.Scan(&t.panel, &t.account, &t.droplet, &t.host, &t.user, &t.keyRef); err != nil {
			return err
		}
		ts = append(ts, t)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, t := range ts {
		t := t
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			s.runOne(ctx, t)
		}()
	}
	wg.Wait()
	return ctx.Err()
}

func (s Service) runOne(ctx context.Context, t target) {
	x := Snapshot{}
	if t.keyRef == "" {
		x.LastError = "missing ssh key"
		s.save(ctx, t.panel, x)
		return
	}
	key, err := s.Secrets.Get(ctx, t.account, t.keyRef)
	if err != nil {
		x.LastError = err.Error()
		s.save(ctx, t.panel, x)
		return
	}
	defer credentials.Wipe(key)
	cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	out, err := s.SSH.Run(cctx, provisioning.Target{AccountID: t.account, DropletID: t.droplet, Host: t.host, User: t.user, KeySecretRef: t.keyRef}, key, guardianCommand(s.Repair))
	if err != nil {
		x.LastError = err.Error()
		s.save(ctx, t.panel, x)
		return
	}
	x = parse(out)
	if !x.XUIActive {
		x.LastError = "x-ui inactive"
	}
	s.save(ctx, t.panel, x)
}

func parse(out string) Snapshot {
	var x Snapshot
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "xui_active":
			x.XUIActive = v == "1"
		case "xui_restarted":
			x.Restarted = v == "1"
		case "memory_available_mb":
			x.MemMB, _ = strconv.ParseInt(v, 10, 64)
		case "disk_free_mb":
			x.DiskMB, _ = strconv.ParseInt(v, 10, 64)
		case "load_1m":
			x.Load, _ = strconv.ParseFloat(v, 64)
		case "package_lock_busy":
			x.LockBusy = v == "1"
		case "reboot_required":
			x.Reboot = v == "1"
		case "db_present":
			x.DBPresent = v == "1"
		case "db_quick_check":
			x.DBCheck = v
		}
	}
	return x
}

func (s Service) save(ctx context.Context, panel string, x Snapshot) {
	if len(x.LastError) > 500 {
		x.LastError = x.LastError[:500]
	}
	_, _ = s.DB.ExecContext(ctx, `INSERT INTO server_runtime_snapshots(panel_id,checked_at,xui_active,xui_restarted,memory_available_mb,disk_free_mb,load_1m,package_lock_busy,reboot_required,db_present,db_quick_check,last_error) VALUES($1,now(),$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(panel_id) DO UPDATE SET checked_at=now(),xui_active=excluded.xui_active,xui_restarted=excluded.xui_restarted,memory_available_mb=excluded.memory_available_mb,disk_free_mb=excluded.disk_free_mb,load_1m=excluded.load_1m,package_lock_busy=excluded.package_lock_busy,reboot_required=excluded.reboot_required,db_present=excluded.db_present,db_quick_check=excluded.db_quick_check,last_error=excluded.last_error`, panel, x.XUIActive, x.Restarted, x.MemMB, x.DiskMB, x.Load, x.LockBusy, x.Reboot, x.DBPresent, x.DBCheck, x.LastError)
}

func guardianCommand(repair bool) string {
	r := "0"
	if repair {
		r = "1"
	}
	return fmt.Sprintf(`set -u
active=0; restarted=0; systemctl is-active x-ui >/dev/null 2>&1 && active=1
if [ "$active" -eq 0 ] && [ "%s" = "1" ]; then systemctl restart x-ui >/dev/null 2>&1 || true; sleep 2; systemctl is-active x-ui >/dev/null 2>&1 && { active=1; restarted=1; }; fi
mem="$(awk '/MemAvailable:/{printf "%%d",$2/1024}' /proc/meminfo 2>/dev/null || echo 0)"
disk="$(df -Pm / | awk 'NR==2{print $4}' 2>/dev/null || echo 0)"
load="$(awk '{print $1}' /proc/loadavg 2>/dev/null || echo 0)"
lock=0; for f in /var/lib/dpkg/lock-frontend /var/lib/dpkg/lock /var/cache/apt/archives/lock; do command -v fuser >/dev/null 2>&1 && fuser "$f" >/dev/null 2>&1 && lock=1; done
reboot=0; [ -f /var/run/reboot-required ] && reboot=1
db=0; check=unavailable; if [ -f /etc/x-ui/x-ui.db ]; then db=1; command -v sqlite3 >/dev/null 2>&1 && check="$(sqlite3 -readonly /etc/x-ui/x-ui.db 'PRAGMA quick_check;' 2>/dev/null | head -1)"; fi
printf 'xui_active=%%s\nxui_restarted=%%s\nmemory_available_mb=%%s\ndisk_free_mb=%%s\nload_1m=%%s\npackage_lock_busy=%%s\nreboot_required=%%s\ndb_present=%%s\ndb_quick_check=%%s\n' "$active" "$restarted" "$mem" "$disk" "$load" "$lock" "$reboot" "$db" "$check"`, r)
}
