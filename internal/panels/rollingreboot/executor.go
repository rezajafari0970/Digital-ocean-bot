package rollingreboot

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/reality/credentials"
)

type Secrets interface {
	Get(context.Context, string, string) ([]byte, error)
}

type Executor struct {
	DB      *sql.DB
	Secrets Secrets
	SSH     provisioning.SSHClient
	Enabled bool
}

type execTarget struct {
	job, panel, account, droplet, host, user, keyRef string
	ready                                            int
}

func (e Executor) RunOne(ctx context.Context) error {
	if !e.Enabled {
		return nil
	}
	t, ok, err := e.claim(ctx)
	if err != nil || !ok {
		return err
	}
	if err = e.execute(ctx, t); err != nil {
		_, _ = e.DB.ExecContext(context.Background(), `UPDATE rolling_reboot_jobs SET state='FAILED',last_error=$2,updated_at=now() WHERE id=$1`, t.job, clip(err.Error(), 500))
		return err
	}
	_, err = e.DB.ExecContext(ctx, `UPDATE rolling_reboot_jobs SET state='COMPLETED',completed_at=now(),updated_at=now(),last_error='' WHERE id=$1`, t.job)
	return err
}

func (e Executor) claim(ctx context.Context) (execTarget, bool, error) {
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return execTarget{}, false, err
	}
	defer tx.Rollback()
	var t execTarget
	err = tx.QueryRowContext(ctx, `
SELECT j.id::text,j.panel_id::text,j.account_id::text,j.droplet_id::text,dep.host,
COALESCE(dep.profile_snapshot->>'ssh_user','root'),COALESCE(dep.profile_snapshot->>'ssh_key_secret_ref',''),
(SELECT count(*) FROM droplets x WHERE x.account_id=j.account_id AND x.state='READY')
FROM rolling_reboot_jobs j
JOIN accounts a ON a.id=j.account_id
JOIN droplets d ON d.id=j.droplet_id
JOIN deployments dep ON dep.droplet_id=d.id
JOIN server_runtime_snapshots rs ON rs.panel_id=j.panel_id
WHERE j.state='PENDING' AND a.enabled=true AND a.provider_state='ACTIVE'
AND d.state='READY' AND dep.state='PANEL_COMPLETE'
AND rs.reboot_required=true AND rs.xui_active=true AND rs.last_error=''
AND (d.expires_at IS NULL OR d.expires_at>now()+interval '30 minutes')
AND (SELECT count(*) FROM rolling_reboot_jobs z WHERE z.state IN ('REBOOT_SENT','WAITING_SSH','VERIFYING'))=0
AND (SELECT count(*) FROM droplets x WHERE x.account_id=j.account_id AND x.state='READY')>=2
ORDER BY (SELECT count(*) FROM droplets x WHERE x.account_id=j.account_id AND x.state='READY') DESC,d.expires_at DESC NULLS LAST,j.id
FOR UPDATE OF j SKIP LOCKED LIMIT 1
`).Scan(&t.job, &t.panel, &t.account, &t.droplet, &t.host, &t.user, &t.keyRef, &t.ready)
	if err == sql.ErrNoRows {
		return execTarget{}, false, nil
	}
	if err != nil {
		return execTarget{}, false, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE rolling_reboot_jobs SET state='REBOOT_SENT',attempts=attempts+1,started_at=COALESCE(started_at,now()),updated_at=now() WHERE id=$1 AND state='PENDING'`, t.job)
	if err != nil {
		return execTarget{}, false, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return execTarget{}, false, nil
	}
	if err = tx.Commit(); err != nil {
		return execTarget{}, false, err
	}
	return t, true, nil
}

func (e Executor) execute(ctx context.Context, t execTarget) error {
	if t.keyRef == "" {
		return fmt.Errorf("missing ssh key")
	}
	key, err := e.Secrets.Get(ctx, t.account, t.keyRef)
	if err != nil {
		return err
	}
	defer credentials.Wipe(key)
	target := provisioning.Target{AccountID: t.account, DropletID: t.droplet, Host: t.host, User: t.user, KeySecretRef: t.keyRef}
	before, err := e.SSH.Run(ctx, target, key, "cat /proc/sys/kernel/random/boot_id")
	if err != nil {
		return fmt.Errorf("pre-reboot boot id: %w", err)
	}
	before = strings.TrimSpace(before)
	if before == "" {
		return fmt.Errorf("empty pre-reboot boot id")
	}
	if _, err = e.DB.ExecContext(ctx, `UPDATE rolling_reboot_jobs SET state='WAITING_SSH',boot_id_before=$2,updated_at=now() WHERE id=$1`, t.job, before); err != nil {
		return err
	}
	// The SSH session may disappear while reboot is being scheduled; boot_id
	// verification below is the authoritative outcome check.
	_, _ = e.SSH.Run(ctx, target, key, "nohup sh -c 'sleep 2; systemctl reboot' >/dev/null 2>&1 & echo scheduled")
	time.Sleep(5 * time.Second)
	waitCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	if err = e.SSH.Wait(waitCtx, target, key); err != nil {
		return fmt.Errorf("ssh did not return: %w", err)
	}
	_, _ = e.DB.ExecContext(ctx, `UPDATE rolling_reboot_jobs SET state='VERIFYING',updated_at=now() WHERE id=$1`, t.job)
	after, err := e.SSH.Run(waitCtx, target, key, "cat /proc/sys/kernel/random/boot_id; systemctl is-active x-ui; test -f /etc/x-ui/x-ui.db && echo db=1 || echo db=0; test -f /var/run/reboot-required && echo reboot_required=1 || echo reboot_required=0")
	if err != nil {
		return fmt.Errorf("post-reboot verify: %w", err)
	}
	lines := strings.Fields(after)
	if len(lines) < 4 {
		return fmt.Errorf("incomplete post-reboot verify")
	}
	if lines[0] == before {
		return fmt.Errorf("boot id unchanged")
	}
	if !strings.Contains(after, "active") || !strings.Contains(after, "db=1") {
		return fmt.Errorf("x-ui/db verification failed: %s", clip(after, 180))
	}
	if strings.Contains(after, "reboot_required=1") {
		return fmt.Errorf("reboot-required still present")
	}
	return nil
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
