#!/usr/bin/env python3
"""Controller adapter for the shared provisioning kernel-memory guard. Never reboots."""
import argparse
import datetime
import json
import os
from pathlib import Path
import subprocess
import tempfile

SHARED = Path(__file__).resolve().parents[1] / "internal/provisioning/kernel_memory_guard.py"
STATE = Path("/var/lib/datacenter-bot-health")

def guard(mode):
    result = subprocess.run(["python3", "-c", "MODE=" + repr(mode) + "\n" + SHARED.read_text()],
                            capture_output=True, text=True, timeout=120,
                            env=dict(os.environ, PATH=os.environ.get("PATH", "") + ":/usr/sbin:/sbin"))
    if result.returncode:
        raise RuntimeError("shared_kernel_guard_failed")
    return result.stdout.strip()

def inspect(mode):
    container = subprocess.run(["systemd-detect-virt", "--container"], capture_output=True,
                               text=True, timeout=10)
    if container.returncode == 0:
        # A container cannot alter or reboot its owner's kernel.
        return {"status": "externally_managed", "reason": "container_kernel_owned_by_host"}
    if container.returncode != 1:
        raise RuntimeError("host_virtualization_unverified")
    if mode == "prepare":
        guard("prepare")
    elif mode == "activation":
        guard("activation")
    report = json.loads(guard("audit"))
    pending = guard("needs-reboot")
    if pending not in ("yes", "no"):
        raise RuntimeError("kernel_activation_unverified")
    report.update(status="needs_reboot" if pending == "yes" else "ok",
                  reason="kernel_memory_reboot_required" if pending == "yes" else "")
    return report

def publish(report):
    STATE.mkdir(mode=0o755, parents=True, exist_ok=True)
    fd, temporary = tempfile.mkstemp(prefix=".host-memory-", dir=STATE)
    try:
        with os.fdopen(fd, "w") as stream:
            os.fchmod(stream.fileno(), 0o644)
            json.dump(report, stream, sort_keys=True)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, STATE / "host-memory.json")
        fd = os.open(STATE, os.O_DIRECTORY)
        try:
            os.fsync(fd)
        finally:
            os.close(fd)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=["audit", "check", "prepare", "activation"], nargs="?", default="audit")
    mode = parser.parse_args().mode
    try:
        report = inspect(mode)
    except Exception as exc:
        report = {"status": "failed", "reason": str(exc) if isinstance(exc, RuntimeError) else "kernel_guard_unavailable"}
    report.update(version=1, checked_at=datetime.datetime.now(datetime.timezone.utc).timestamp())
    if mode != "audit" and os.geteuid() == 0:
        publish(report)
    print(json.dumps(report, sort_keys=True))
    if report["status"] == "failed" or (mode in ("check", "activation") and report["status"] == "needs_reboot"):
        raise SystemExit(2)

if __name__ == "__main__":
    main()
