package provisioning

import _ "embed"

// KernelMemoryGuardCommand stages the documented KHO workaround only for
// susceptible kernels. It never reboots; installer readiness and explicit
// serial maintenance own activation. Existing boot arguments are preserved.
func KernelMemoryGuardCommand() string           { return kernelMemoryGuard("prepare") }
func KernelMemoryGuardReadyCommand() string      { return kernelMemoryGuard("check") }
func KernelMemoryGuardAuditCommand() string      { return kernelMemoryGuard("audit") }
func KernelMemoryGuardActivationCommand() string { return kernelMemoryGuard("activation") }
func KernelMemoryRebootRequiredCommand() string {
	return "if test -f /var/run/reboot-required; then echo yes; else\n" + kernelMemoryGuard("needs-reboot") + "\nfi"
}
func kernelMemoryGuard(mode string) string {
	return "python3 - <<'PYKHOGUARD'\nMODE='" + mode + "'\n" + kernelMemoryGuardPython + "\nPYKHOGUARD"
}

//go:embed kernel_memory_guard.py
var kernelMemoryGuardPython string
