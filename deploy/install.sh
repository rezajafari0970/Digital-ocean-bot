#!/usr/bin/env bash
set -euo pipefail
[ "$(id -u)" -eq 0 ] || { echo 'run as root'; exit 1; }
SRC="${SRC:-$(cd "$(dirname "$0")/.." && pwd)}"
APP=/opt/digital-ocean-bot
ETC=/etc/digital-ocean-bot
DATA=/var/lib/digital-ocean-bot
DB_NAME=${DB_NAME:-digital_ocean_bot}
DB_USER=${DB_USER:-digitaloceanbot}

id -u digitaloceanbot >/dev/null 2>&1 || useradd --system --home "$DATA" --shell /usr/sbin/nologin digitaloceanbot
install -d -o digitaloceanbot -g digitaloceanbot -m 0750 "$APP/bin" "$DATA" "$DATA/templates"
install -d -o root -g digitaloceanbot -m 0750 "$ETC"
cp -a "$SRC/migrations" "$APP/"
cd "$SRC"
go build -trimpath -ldflags='-s -w' -o "$APP/bin/digital-ocean-bot-api" ./cmd/api
go build -trimpath -ldflags='-s -w' -o "$APP/bin/digital-ocean-bot-worker" ./cmd/worker
chown root:digitaloceanbot "$APP/bin/"*; chmod 0750 "$APP/bin/"*

if [ ! -f "$ETC/master.key" ]; then umask 077; openssl rand -base64 32 > "$ETC/master.key"; chown root:digitaloceanbot "$ETC/master.key"; chmod 0640 "$ETC/master.key"; fi
if [ ! -f "$ETC/env" ]; then
  DB_PASS=$(openssl rand -hex 24)
  sudo -u postgres psql -v ON_ERROR_STOP=1 <<SQL
DO \$\$ BEGIN IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='$DB_USER') THEN CREATE ROLE $DB_USER LOGIN PASSWORD '$DB_PASS'; END IF; END \$\$;
SELECT 'CREATE DATABASE $DB_NAME OWNER $DB_USER' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname='$DB_NAME')\gexec
SQL
  cat > "$ETC/env" <<EOF
DATABASE_URL=postgres://$DB_USER:$DB_PASS@127.0.0.1:5432/$DB_NAME?sslmode=disable
MASTER_KEY_FILE=$ETC/master.key
MASTER_KEY_VERSION=1
HTTP_ADDR=127.0.0.1:18080
EOF
  chown root:digitaloceanbot "$ETC/env"; chmod 0640 "$ETC/env"
fi
install -m 0644 "$SRC/deploy/digital-ocean-bot-api.service" /etc/systemd/system/
install -m 0644 "$SRC/deploy/digital-ocean-bot-worker.service" /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now digital-ocean-bot-api digital-ocean-bot-worker
systemctl --no-pager --full status digital-ocean-bot-api digital-ocean-bot-worker || true
