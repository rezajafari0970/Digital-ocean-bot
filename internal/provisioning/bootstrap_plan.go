package provisioning

import "time"

func DefaultBootstrapSteps() []ScriptStep {
	return []ScriptStep{
		{Name: "bootstrap-cloud-init", Category: "bootstrap", Precheck: `! command -v cloud-init >/dev/null 2>&1 || test -f /var/lib/cloud/instance/boot-finished`, Execute: `cloud-init status --wait`, Verify: `! command -v cloud-init >/dev/null 2>&1 || test -f /var/lib/cloud/instance/boot-finished`, Timeout: 8 * time.Minute, MaxAttempts: 4},
		{Name: "bootstrap-package-state", Category: "bootstrap", Precheck: `if command -v dpkg >/dev/null 2>&1; then test -z "$(dpkg --audit 2>&1)"; else true; fi`, Execute: `if command -v dpkg >/dev/null 2>&1; then DEBIAN_FRONTEND=noninteractive dpkg --configure -a; fi`, Verify: `if command -v dpkg >/dev/null 2>&1; then test -z "$(dpkg --audit 2>&1)"; else true; fi`, Timeout: 5 * time.Minute, MaxAttempts: 3},
		{Name: "bootstrap-package-index", Category: "bootstrap", Precheck: `if command -v apt-get >/dev/null 2>&1; then find /var/lib/apt/lists -type f -mmin -360 -print -quit 2>/dev/null | grep -q .; else true; fi`, Execute: `set -e; if command -v apt-get >/dev/null 2>&1; then DEBIAN_FRONTEND=noninteractive apt-get update -y; elif command -v dnf >/dev/null 2>&1; then dnf -y makecache; elif command -v yum >/dev/null 2>&1; then yum -y makecache; elif command -v apk >/dev/null 2>&1; then apk update; fi`, Verify: `if command -v apt-get >/dev/null 2>&1; then test -d /var/lib/apt/lists && find /var/lib/apt/lists -type f -print -quit | grep -q .; else true; fi`, Timeout: 10 * time.Minute, MaxAttempts: 4},
		{Name: "bootstrap-prerequisites", Category: "bootstrap", Precheck: `command -v curl >/dev/null 2>&1 && command -v wget >/dev/null 2>&1 && test -r /etc/ssl/certs/ca-certificates.crt`, Execute: `set -e; if command -v apt-get >/dev/null 2>&1; then DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl wget; elif command -v dnf >/dev/null 2>&1; then dnf install -y ca-certificates curl wget; elif command -v yum >/dev/null 2>&1; then yum install -y ca-certificates curl wget; elif command -v apk >/dev/null 2>&1; then apk add --no-cache ca-certificates curl wget; fi`, Verify: `command -v curl >/dev/null 2>&1 && command -v wget >/dev/null 2>&1 && test -r /etc/ssl/certs/ca-certificates.crt`, Timeout: 10 * time.Minute, MaxAttempts: 4},
		{Name: "bootstrap-verify", Category: "bootstrap", Verify: `set -e; test "$(id -u)" = 0; getent hosts deb.debian.org >/dev/null; if command -v dpkg >/dev/null 2>&1; then test -z "$(dpkg --audit 2>&1)"; fi; command -v curl >/dev/null; curl -fsSIL --max-time 10 https://deb.debian.org >/dev/null`, Timeout: 2 * time.Minute, MaxAttempts: 3},
	}
}

func DefaultPreInstallerSteps() []ScriptStep {
	steps := append([]ScriptStep{}, DefaultBootstrapSteps()...)
	steps = append(steps,
		ScriptStep{Name: "panel", Category: "install", Execute: "true", MaxAttempts: 1, Timeout: time.Minute},
		ScriptStep{Name: "verify", Category: "verify", Execute: "true", MaxAttempts: 1, Timeout: time.Minute},
	)
	return steps
}
