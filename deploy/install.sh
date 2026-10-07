#!/usr/bin/env bash
set -Eeuo pipefail
[ "$(id -u)" -eq 0 ] || { echo 'run as root'; exit 1; }
SRC="${SRC:-$(cd "$(dirname "$0")/.." && pwd)}"
APP=/opt/digital-ocean-bot
source "$SRC/deploy/service-topology.sh"
dob_preflight_service_topology
dob_acquire_deploy_lock
BUILD_COMMIT="$(git -C "$SRC" rev-parse HEAD 2>/dev/null || echo unknown)"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-s -w -X github.com/rezajafari0970/Digital-ocean-bot/internal/buildinfo.Commit=$BUILD_COMMIT -X github.com/rezajafari0970/Digital-ocean-bot/internal/buildinfo.BuildTime=$BUILD_TIME"
ETC=/etc/digital-ocean-bot
DATA=/var/lib/digital-ocean-bot
DB_NAME=${DB_NAME:-digital_ocean_bot}
DB_USER=${DB_USER:-digitaloceanbot}

id -u digitaloceanbot >/dev/null 2>&1 || useradd --system --home "$DATA" --shell /usr/sbin/nologin digitaloceanbot
dob_stage_runtime
dob_prepare_stage_permissions
install -d -o digitaloceanbot -g digitaloceanbot -m 0750 "$DATA" "$DATA/templates"
install -d -o digitaloceanbot -g digitaloceanbot -m 0700 "$DATA/browser-sessions"
install -d -o root -g digitaloceanbot -m 0750 "$ETC"

# Atomic, validated bootstrap artifacts precede all SQL and runtime changes.
python3 "$SRC/deploy/bootstrap.py" "$ETC" "$DB_NAME" "$DB_USER" "$(id -g digitaloceanbot)"
if [ -f "$ETC/bootstrap.pending" ]; then
  mapfile -t DB_IDENTITY < "$ETC/bootstrap.pending"
  DB_NAME=${DB_IDENTITY[0]}; DB_USER=${DB_IDENTITY[1]}
  [[ "$DB_NAME" =~ ^[A-Za-z_][A-Za-z0-9_]*$ && "$DB_USER" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || exit 1
  DATABASE_URL=$(python3 "$SRC/deploy/bootstrap.py" --database-url "$ETC")
  DB_PASS=${DATABASE_URL#postgres://$DB_USER:}
  DB_PASS=${DB_PASS%%@*}
  [[ "$DB_PASS" =~ ^[a-f0-9]{48}$ ]] || { echo 'Bootstrap credential format mismatch'; exit 1; }
  test "$DATABASE_URL" = "postgres://$DB_USER:$DB_PASS@127.0.0.1:5432/$DB_NAME?sslmode=disable"
  sudo -u postgres psql -v ON_ERROR_STOP=1 <<SQL
DO \$\$ BEGIN IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='$DB_USER') THEN CREATE ROLE $DB_USER LOGIN PASSWORD '$DB_PASS'; END IF; END \$\$;
SELECT 'CREATE DATABASE $DB_NAME OWNER $DB_USER' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname='$DB_NAME')\gexec
SQL
  psql "$DATABASE_URL" -XAt -v ON_ERROR_STOP=1 -c 'SELECT 1' >/dev/null
  rm -f "$ETC/bootstrap.pending"
fi
# Bootstrap state is retained on release failure for a retry with the same
# master key/database identity. Never delete credentials for an existing DB.
dob_backup_runtime
trap dob_rollback_runtime ERR
dob_stop_runtime_readers
dob_publish_runtime

dob_install_service_topology
systemctl enable "${SERVICES[@]}"
# Always restart: enable --now does not restart already-active services after replacing binaries.
systemctl restart "${SERVICES[@]}"
dob_verify_service_topology
trap - ERR
echo "Deployment verified. Rollback artifacts: $DOB_BACKUP"
