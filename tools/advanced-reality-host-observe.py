#!/usr/bin/env python3
"""Read-only host census. No packet capture, connection addresses or writes.

All counters describe the entire host, not an inbound or a VPN user. Compare
two samples from the same boot to obtain rates; never equate retransmissions
with end-to-end packet loss. Run through the existing authorized host channel.
"""
import datetime
import hashlib
import json
import pathlib
import platform
import subprocess


def read(path):
    try:
        return pathlib.Path(path).read_text().strip()
    except (OSError, UnicodeError):
        return None


def command(args):
    try:
        p = subprocess.run(args, capture_output=True, text=True, timeout=3)
        return p.stdout.strip() if p.returncode == 0 else None
    except (OSError, subprocess.TimeoutExpired):
        return None


def census():
    names = ["net.ipv4.tcp_available_congestion_control", "net.ipv4.tcp_congestion_control",
             "net.core.default_qdisc", "net.ipv4.tcp_keepalive_time", "net.ipv4.tcp_keepalive_intvl",
             "net.ipv4.tcp_keepalive_probes", "net.ipv4.tcp_rmem", "net.ipv4.tcp_wmem",
             "net.core.somaxconn", "net.ipv4.tcp_max_syn_backlog", "fs.file-max"]
    sysctls = {n: read("/proc/sys/" + n.replace(".", "/")) for n in names}
    tcp = {}
    lines = (read("/proc/net/snmp") or "").splitlines()
    wanted = {"ActiveOpens", "PassiveOpens", "AttemptFails", "EstabResets", "CurrEstab",
              "InSegs", "OutSegs", "RetransSegs", "InErrs", "OutRsts"}
    for i in range(len(lines)-1):
        if lines[i].startswith("Tcp:") and "ActiveOpens" in lines[i]:
            for name, val in zip(lines[i].split()[1:], lines[i+1].split()[1:]):
                if name in wanted:
                    try:
                        tcp[name] = int(val)
                    except ValueError:
                        tcp[name] = None
    memory = {}
    for line in (read("/proc/meminfo") or "").splitlines():
        k, _, v = line.partition(":")
        if k in {"MemTotal", "MemAvailable", "SwapTotal", "SwapFree"}:
            memory[k+"_kib"] = int(v.strip().split()[0])
    unit = command(["systemctl", "show", "x-ui.service", "--no-pager",
                    "-p", "LoadState", "-p", "ActiveState", "-p", "SubState", "-p", "NRestarts",
                    "-p", "MemoryCurrent", "-p", "TasksCurrent", "-p", "LimitNOFILE"])
    boot = read("/proc/sys/kernel/random/boot_id")
    return {
        "schema": 1, "mode": "READ_ONLY", "scope": "whole_host_not_per_inbound",
        "observed_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "kernel": platform.release(), "architecture": platform.machine(),
        "os": {k: v.strip('"') for line in (read("/etc/os-release") or "").splitlines()
               for k, sep, v in [line.partition("=")] if sep and k in {"ID", "VERSION_ID"}},
        "boot_hash": hashlib.sha256(boot.encode()).hexdigest() if boot else None,
        "uptime_seconds": (read("/proc/uptime") or "").split(" ")[0] or None,
        "sysctls": sysctls, "tcp_counters": tcp, "memory": memory,
        "pressure": {k: read("/proc/pressure/"+k) for k in ["cpu", "memory", "io"]},
        "file_handles": read("/proc/sys/fs/file-nr"),
        "service": dict(line.split("=", 1) for line in unit.splitlines() if "=" in line) if unit else None,
        "unmeasured": ["per_inbound_cpu", "per_stream_memory", "client_path_rtt", "packet_loss",
                       "tls_failures", "reality_failures", "http2_streams", "queue_depth"],
        "production_changed": False,
    }


if __name__ == "__main__":
    print(json.dumps(census(), sort_keys=True))
