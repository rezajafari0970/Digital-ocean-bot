#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
[ "$(id -u)" = 0 ] || exit 1
SRC=$(cd "$(dirname "$0")/.." && pwd)
APP=/opt/digital-ocean-bot
ETC=/etc/digital-ocean-bot
DATA=/var/lib/digital-ocean-bot
MODE=${1:-install}
source "$SRC/deploy/service-topology.sh"
dob_preflight_service_topology
install -d -m 0755 /opt
dob_acquire_deploy_lock
if [ -f "$ETC/deployment.pending" ]; then
 echo 'An interrupted deployment needs recovery: sudo datacenter-bot recover' >&2; exit 1
fi
echo '[3/7] Installing system packages and verified Go toolchain'
export DEBIAN_FRONTEND=noninteractive
apt-get -o DPkg::Lock::Timeout=180 update
apt-get -o DPkg::Lock::Timeout=180 install -y --no-install-recommends ca-certificates curl git python3 postgresql postgresql-client sudo openssh-client util-linux procps iproute2 sqlite3 xz-utils
systemctl enable --now postgresql
source "$SRC/deploy/toolchain.sh"
dob_install_toolchain
BUILD_COMMIT=$(git -C "$SRC" rev-parse HEAD)
BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS="-s -w -X github.com/rezajafari0970/Digital-ocean-bot/internal/buildinfo.Commit=$BUILD_COMMIT -X github.com/rezajafari0970/Digital-ocean-bot/internal/buildinfo.BuildTime=$BUILD_TIME"
echo '[4/7] Building the complete release before touching running services'
id -u digitaloceanbot >/dev/null 2>&1 || useradd --system --home "$DATA" --shell /usr/sbin/nologin digitaloceanbot
dob_stage_runtime
dob_prepare_stage_permissions
install -d -o digitaloceanbot -g digitaloceanbot -m 0750 "$DATA" "$DATA/templates"
install -d -o digitaloceanbot -g digitaloceanbot -m 0700 "$DATA/browser-sessions"
install -d -o root -g digitaloceanbot -m 0750 "$ETC"
python3 "$SRC/deploy/install-state.py" prepare
python3 "$SRC/deploy/install-state.py" backup
echo '[5/7] Publishing, applying migrations and preserving users'
dob_backup_runtime
DOB_PENDING="$ETC/deployment.pending"
printf '%s\n%s\n' "$SRC" "$DOB_BACKUP" > "$DOB_PENDING"
chmod 0600 "$DOB_PENDING"
trap dob_rollback_runtime ERR
dob_stop_runtime_readers
if systemctl is-active --quiet digital-ocean-bot-vultr-browser-manager; then systemctl stop digital-ocean-bot-vultr-browser-manager; fi
systemctl disable digital-ocean-bot-vultr-browser-manager 2>/dev/null || true
dob_publish_runtime
python3 "$SRC/deploy/install-state.py" bootstrap "$APP"
dob_install_service_topology
echo '[6/7] Enabling persistent services and checking readiness'
systemctl enable "${SERVICES[@]}"
systemctl start "${SERVICES[@]}"
dob_verify_service_topology
python3 "$SRC/deploy/install-state.py" complete
rm -f "$DOB_PENDING"
trap - ERR
install -d -m 0755 /usr/local/lib/datacenter-bot
install -m 0755 "$SRC/install.sh" /usr/local/lib/datacenter-bot/install.sh
install -m 0755 "$SRC/deploy/datacenter-bot" /usr/local/bin/datacenter-bot
printf '%s\n' "$SRC" > /usr/local/lib/datacenter-bot/source
chmod 0600 /usr/local/lib/datacenter-bot/source
echo '[7/7] Installation/update verified'
echo "Installed revision: $BUILD_COMMIT"
echo 'Update command: sudo datacenter-bot update'
python3 - "$SRC" <<'PY'
import json,pathlib,subprocess,sys,runpy,os
src=pathlib.Path(sys.argv[1])
d=json.loads(subprocess.check_output(['python3',str(src/'deploy/install-state.py'),'address'],text=True))
host=subprocess.check_output(['hostname','-I'],text=True).split()[0]
url='http://'+host+':'+str(d['port'])+d['path']
print('Panel: '+url)
grant=pathlib.Path('/etc/digital-ocean-bot/setup-token')
values=runpy.run_path(str(src/'deploy/bootstrap.py'))['read_environment'](pathlib.Path('/etc/digital-ocean-bot/env'))
p=subprocess.run(['psql','-XAt','-c',"SELECT count(*) FROM admin_users WHERE must_change_password AND bootstrap_token_hash IS NOT NULL"],env=runpy.run_path(str(src/'deploy/bootstrap.py'))['database_environment'](values),capture_output=True,text=True)
if p.returncode==0 and p.stdout.strip()!='0' and grant.exists():
 print('Private first-use link: '+url+'#setup='+grant.read_text().strip())
 print('Initial username/password: admin / admin. Set a new password at first login.')
 print('Use this private link over an SSH tunnel or your HTTPS reverse proxy.')
PY
