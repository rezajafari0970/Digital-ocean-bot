package sanaei

func VerifyCommand() string {
	return `set -euo pipefail; systemctl is-active x-ui >/dev/null; command -v x-ui >/dev/null || test -x /usr/local/x-ui/x-ui`
}

func shellQuote(s string) string { return "'" + s + "'" }
