package sanaei

func VerifyCommand() string {
	return `set -euo pipefail; systemctl is-active x-ui >/dev/null; (command -v x-ui >/dev/null || test -x /usr/local/x-ui/x-ui); test -f /etc/x-ui/x-ui.db`
}

func shellQuote(s string) string { return "'" + s + "'" }
