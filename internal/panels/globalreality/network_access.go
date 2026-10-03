package globalreality

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"strings"
)

// Open only the configured client ports and the management port from the SSH
// control peer. Never disable a firewall or broaden management access.
func networkAccessCommand(panelPort int, ports []int) (string, error) {
	if panelPort < 1 || panelPort > 65535 {
		return "", errors.New("invalid panel port")
	}
	for _, p := range ports {
		if p < 1 || p > 65535 || p == panelPort || p == 22 {
			return "", errors.New("client port conflicts with management")
		}
	}
	raw, _ := json.Marshal(ports)
	cmd := `set -eu
python3 - <<'PYNETWORK'
import ipaddress,json,os,subprocess,shlex
panel_port=PANEL_PORT
ports=CLIENT_PORTS
peer=str(ipaddress.ip_address(os.environ.get('SSH_CONNECTION','').split()[0]))
def run(*args):
 return subprocess.run(args,check=True,capture_output=True,text=True).stdout
ufw=subprocess.run(['sh','-c','command -v ufw'],capture_output=True,text=True).returncode==0
if ufw and 'Status: active' in run('ufw','status'):
 # UFW commands are idempotent, but inspect the committed rules first on every retry.
 def allow(args):
  before=run('ufw','show','added')
  desired=['ufw',*args]
  if not any(shlex.split(line)==desired for line in before.splitlines()):
   run('ufw',*args)
  after=run('ufw','show','added')
  if not any(shlex.split(line)==desired for line in after.splitlines()): raise RuntimeError('firewall rule not verified')
 allow(['allow','from',peer,'to','any','port',str(panel_port),'proto','tcp','comment','dob-control'])
 for port in sorted(set(ports)):
  allow(['allow',str(port)+'/tcp','comment','dob-reality'])
elif subprocess.run(['sh','-c','command -v firewall-cmd >/dev/null && firewall-cmd --state >/dev/null'],capture_output=True).returncode==0:
 raise RuntimeError('unsupported active firewall; explicit rules required')
print('scoped network access verified')
PYNETWORK`
	cmd = strings.ReplaceAll(cmd, "PANEL_PORT", fmt.Sprint(panelPort))
	cmd = strings.ReplaceAll(cmd, "CLIENT_PORTS", string(raw))
	return cmd, nil
}
func (s Service) ensureNetworkAccess(ctx context.Context, panel string, ports []int) error {
	var acc, did, host, user, ref string
	var panelPort int
	err := s.DB.QueryRowContext(ctx, `SELECT pi.account_id::text,pi.droplet_id::text,d.host,COALESCE(d.profile_snapshot->>'ssh_user','root'),
 COALESCE(d.profile_snapshot->>'ssh_key_secret_ref',''),x.port FROM panel_instances pi
 JOIN deployments d ON d.droplet_id=pi.droplet_id JOIN xui_panel_deployments x ON x.droplet_id=pi.droplet_id AND x.generation=d.postinstall_generation
 JOIN droplets dr ON dr.id=pi.droplet_id JOIN accounts a ON a.id=pi.account_id
 WHERE pi.id=$1 AND pi.enabled AND a.enabled AND a.deletion_requested_at IS NULL AND dr.state='READY'`, panel).Scan(&acc, &did, &host, &user, &ref, &panelPort)
	if err != nil {
		return err
	}
	command, err := networkAccessCommand(panelPort, ports)
	if err != nil {
		return err
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(host+command)))
	var fresh bool
	if err = s.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM panel_network_access WHERE panel_id=$1 AND plan_hash=$2 AND verified_at>now()-interval '5 minutes')", panel, hash).Scan(&fresh); err != nil || fresh {
		return err
	}
	key, err := s.Secrets.Get(ctx, acc, ref)
	if err != nil {
		return err
	}
	defer func() {
		for i := range key {
			key[i] = 0
		}
	}()
	_, err = s.SSH.Run(ctx, provisioning.Target{AccountID: acc, DropletID: did, Host: host, Port: 22, User: user, KeySecretRef: ref}, key, command)
	if err != nil {
		return fmt.Errorf("scoped panel network access: %w", err)
	}
	_, err = s.DB.ExecContext(ctx, "INSERT INTO panel_network_access(panel_id,plan_hash) VALUES($1,$2) ON CONFLICT(panel_id) DO UPDATE SET plan_hash=excluded.plan_hash,verified_at=now()", panel, hash)
	return err
}
