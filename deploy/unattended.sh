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
python3 "$SRC/deploy/host-memory.py" check
python3 "$SRC/deploy/disk-preflight.py" build
echo '[3/7] Installing system packages and verified Go toolchain'
export DEBIAN_FRONTEND=noninteractive
apt-get -o DPkg::Lock::Timeout=180 update
AUDIO_PACKAGE=libasound2; ATK_PACKAGE=libatk-bridge2.0-0; CUPS_PACKAGE=libcups2
if apt-cache show libasound2t64 >/dev/null 2>&1; then AUDIO_PACKAGE=libasound2t64; fi
if apt-cache show libatk-bridge2.0-0t64 >/dev/null 2>&1; then ATK_PACKAGE=libatk-bridge2.0-0t64; fi
if apt-cache show libcups2t64 >/dev/null 2>&1; then CUPS_PACKAGE=libcups2t64; fi
# Include the browser's own shared libraries: development/CI hosts already have
# many of these, while a fresh minimal Ubuntu installation does not.
apt-get -o DPkg::Lock::Timeout=180 install -y --no-install-recommends ca-certificates curl git python3 postgresql postgresql-client sudo openssh-client util-linux procps iproute2 sqlite3 xz-utils $AUDIO_PACKAGE $ATK_PACKAGE $CUPS_PACKAGE libnss3 libnspr4 libgbm1 libdrm2 libx11-6 libxcb1 libxcomposite1 libxdamage1 libxext6 libxfixes3 libxrandr2 libxkbcommon0 libpango-1.0-0 libcairo2 libdbus-1-3 fonts-liberation
systemctl enable --now postgresql
source "$SRC/deploy/toolchain.sh"
dob_install_toolchain
python3 "$SRC/deploy/provider-browser-runtime.py"
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
# Inspect while the previous release still serves requests.
python3 "$SRC/deploy/database-integrity.py"
python3 "$SRC/deploy/disk-preflight.py" publish --etc "$ETC"
echo '[5/7] Publishing, applying migrations and preserving users'
dob_backup_runtime
DOB_PENDING="$ETC/deployment.pending"
printf '%s\n%s\n' "$SRC" "$DOB_BACKUP" > "$DOB_PENDING"
chmod 0600 "$DOB_PENDING"
trap dob_rollback_runtime ERR
dob_stop_runtime_readers
if systemctl is-active --quiet digital-ocean-bot-vultr-browser-manager; then systemctl stop digital-ocean-bot-vultr-browser-manager; fi
systemctl disable digital-ocean-bot-vultr-browser-manager 2>/dev/null || true
python3 "$SRC/deploy/install-state.py" backup
dob_publish_runtime
python3 "$SRC/deploy/install-state.py" bootstrap "$APP"
dob_install_service_topology
install -d -m 0755 /usr/local/lib/datacenter-bot
install -m 0755 "$SRC/install.sh" /usr/local/lib/datacenter-bot/install.sh
install -m 0755 "$SRC/deploy/datacenter-bot" /usr/local/bin/datacenter-bot
printf '%s\n' "$SRC" > /usr/local/lib/datacenter-bot/source
chmod 0600 /usr/local/lib/datacenter-bot/source
echo '[6/7] Enabling persistent services and checking readiness'
systemctl enable "${SERVICES[@]}"
systemctl start "${SERVICES[@]}"
dob_verify_service_topology
python3 "$SRC/deploy/install-state.py" complete
rm -f "$DOB_PENDING"
trap - ERR
systemctl enable --now datacenter-bot-integrity.timer datacenter-bot-host-memory.timer
# Retention is post-commit: a cleanup error must never roll back a healthy release.
python3 "$SRC/deploy/runtime-retention.py" "$DOB_BACKUP" || echo 'Runtime retention needs attention; copies were preserved.' >&2
python3 "$SRC/deploy/install-state.py" prune-backups || echo 'Backup retention needs attention; existing copies were preserved.' >&2
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
print('Fresh installation login: admin / admin. Existing credentials are preserved on update.')
PY
