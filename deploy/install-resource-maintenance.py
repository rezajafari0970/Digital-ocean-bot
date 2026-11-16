#!/usr/bin/env python3
"""Install project-scoped, bounded archival after production acceptance."""
import os
from pathlib import Path
import shutil
import subprocess

def main():
    if os.geteuid()!=0:
        raise SystemExit("root_required")
    if not Path("/opt/dob-production/build-manifest.json").is_file():
        raise SystemExit("production_instance_required")
    target=Path("/usr/local/lib/dob-resource-maintenance")
    if target.is_symlink():
        raise SystemExit("unowned_install_path")
    target.mkdir(mode=0o755,exist_ok=True)
    source=Path(__file__).with_name("rollback-archive.py")
    temporary=target/"rollback-archive.py.new"
    if temporary.is_symlink():
        raise SystemExit("unowned_staging_path")
    shutil.copyfile(source,temporary)
    temporary.chmod(0o755)
    os.replace(temporary,target/"rollback-archive.py")
    units={
    "service":"""[Unit]
Description=Digital Ocean verified historical runtime archival
After=local-fs.target
[Service]
Type=oneshot
ExecStart=/usr/bin/python3 /usr/local/lib/dob-resource-maintenance/rollback-archive.py --apply
User=root
UMask=0077
Nice=19
IOSchedulingClass=idle
CPUWeight=10
CPUQuota=25%
MemoryMax=128M
TimeoutStartSec=10min
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/opt /var/lib/dob-resource-maintenance /run/lock
StateDirectory=dob-resource-maintenance
StateDirectoryMode=0700
""",
    "timer":"""[Unit]
Description=Digital Ocean historical runtime archive schedule
[Timer]
OnBootSec=15min
OnUnitActiveSec=1h
RandomizedDelaySec=5min
Persistent=yes
Unit=dob-resource-maintenance.service
[Install]
WantedBy=timers.target
"""}
    for extension,body in units.items():
        path=Path("/etc/systemd/system")/("dob-resource-maintenance."+extension)
        if path.is_symlink():
            raise SystemExit("unowned_unit_path")
        path.write_text(body)
        path.chmod(0o644)
    subprocess.run(["systemctl","daemon-reload"],check=True)
    subprocess.run(["systemctl","enable","--now","dob-resource-maintenance.timer"],check=True)

if __name__=="__main__":main()
