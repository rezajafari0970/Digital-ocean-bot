package provisioning

// These exact read-only predicates were stored in immutable Sanaei installer
// script versions 1 and 2. systemctl is-active returns 3 for an inactive unit
// and 4 for an absent unit. Either is an unmet installation predicate here,
// unlike a shell syntax/lookup fault. Do not extend this to arbitrary scripts
// or prefix matching: later commands could supply unrelated exit statuses.
// See https://github.com/systemd/systemd/blob/main/man/systemctl.xml
const legacySanaeiPrecheckV1 = `set -euo pipefail; systemctl is-active x-ui >/dev/null; command -v x-ui >/dev/null || test -x /usr/local/x-ui/x-ui`
const legacySanaeiPrecheckV2 = `set -euo pipefail; systemctl is-active x-ui >/dev/null; (command -v x-ui >/dev/null || test -x /usr/local/x-ui/x-ui); test -f /etc/x-ui/x-ui.db`

func precheckMissExit(command string, code int, stderr string) bool {
	legacy := command == legacySanaeiPrecheckV1 || command == legacySanaeiPrecheckV2
	if legacy && stderr != "" {
		return false
	}
	if code == 1 {
		return true
	}
	if code != 3 && code != 4 {
		return false
	}
	return legacy
}
