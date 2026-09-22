package sanaei

import "fmt"

type DatabasePaths struct {
	Live      string
	Incoming  string
	BackupDir string
}

func DefaultDatabasePaths() DatabasePaths {
	return DatabasePaths{Live: "/etc/x-ui/x-ui.db", Incoming: "/etc/x-ui/x-ui.db.incoming", BackupDir: "/etc/x-ui/backups"}
}

func ImportCommand(paths DatabasePaths, expectedSHA string) string {
	return fmt.Sprintf(`set -euo pipefail
live=%s; incoming=%s; backup_dir=%s
mkdir -p "$backup_dir"
test -f "$incoming"
echo "%s  $incoming" | sha256sum -c -
systemctl stop x-ui
backup="$backup_dir/x-ui.db.$(date +%%s).bak"
if test -f "$live"; then cp -a "$live" "$backup"; fi
if ! mv -f "$incoming" "$live"; then systemctl start x-ui || true; exit 1; fi
chmod 600 "$live"
if ! systemctl start x-ui; then test -f "$backup" && cp -a "$backup" "$live"; systemctl start x-ui || true; exit 1; fi
if ! systemctl is-active x-ui >/dev/null; then systemctl stop x-ui || true; test -f "$backup" && cp -a "$backup" "$live"; systemctl start x-ui || true; exit 1; fi`, shellQuote(paths.Live), shellQuote(paths.Incoming), shellQuote(paths.BackupDir), expectedSHA)
}
