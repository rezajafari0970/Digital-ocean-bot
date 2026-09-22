package sanaei

import "fmt"

type InstallProfile struct {
	ScriptURL string
	Version   string
}

func (p InstallProfile) Command() (string, error) {
	if p.ScriptURL == "" {
		return "", fmt.Errorf("missing installer URL")
	}
	cmd := `set -euo pipefail; export DEBIAN_FRONTEND=noninteractive; tmp=$(mktemp); trap 'rm -f "$tmp"' EXIT; curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 ` + shellQuote(p.ScriptURL) + ` -o "$tmp"; bash "$tmp"`
	return cmd, nil
}

func VerifyCommand() string {
	return `set -euo pipefail; systemctl is-active x-ui >/dev/null; command -v x-ui >/dev/null || test -x /usr/local/x-ui/x-ui`
}

func shellQuote(s string) string { return "'" + s + "'" }
