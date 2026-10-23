#!/usr/bin/env python3
"""Install independent protection jobs after root-owned credentials are provisioned."""
from pathlib import Path
import argparse, os, shutil, subprocess
p=argparse.ArgumentParser()
p.add_argument("--role",choices=["client","vault"],required=True)
args=p.parse_args()
if os.geteuid()!=0: raise SystemExit("Run with sudo.")
source=Path(__file__).resolve().parent
cfg=Path("/etc/datacenter-host-protection")
required=["client.env","restic-password","vault.crt"] if args.role=="client" else ["restic-password"]
for name in required:
 path=cfg/name
 if not path.is_file() or path.is_symlink() or path.stat().st_uid!=0 or path.stat().st_mode & 0o077:
  raise SystemExit("Root-owned private protection credentials are required.")
target=Path("/usr/local/lib/datacenter-host-protection")
target.mkdir(mode=0o755,exist_ok=True)
shutil.copy2(source/"host-protection.py",target/"host-protection.py")
os.chmod(target/"host-protection.py",0o755)
link=Path("/usr/local/sbin/datacenter-host-protection")
if link.exists() or link.is_symlink():
 if not link.is_symlink() or link.resolve()!=target/"host-protection.py": raise SystemExit("Unrelated command exists.")
else: link.symlink_to(target/"host-protection.py")
jobs={"evidence":("5min","1min"),"backup":("2h","5min")} if args.role=="client" else {"retain":("24h","30min")}
for operation,(interval,boot) in jobs.items():
 unit="datacenter-host-"+operation
 identity = ("User=dob-backup-vault\nGroup=dob-backup-vault\nLoadCredential=restic-password:/etc/datacenter-host-protection/restic-password\nStateDirectory=datacenter-backup-maintenance\nStateDirectoryMode=0700" if args.role=="vault" else "User=root")
 service=f"""[Unit]
Description=Independent Datacenter host protection: {operation}
After=network-online.target
[Service]
Type=oneshot
ExecStart=/usr/local/sbin/datacenter-host-protection {operation}
{identity}
UMask=0077
Nice=10
IOSchedulingClass=best-effort
IOSchedulingPriority=7
CPUQuota=100%
MemoryMax=768M
TimeoutStartSec=45min
NoNewPrivileges=yes
PrivateTmp=yes
ProtectKernelTunables=yes
"""
 timer=f"""[Unit]
Description=Independent Datacenter {operation} schedule
[Timer]
OnBootSec={boot}
OnUnitActiveSec={interval}
RandomizedDelaySec=30s
Unit={unit}.service
[Install]
WantedBy=timers.target
"""
 for ext,content in [("service",service),("timer",timer)]:
  path=Path("/etc/systemd/system")/(unit+"."+ext)
  path.write_text(content);os.chmod(path,0o644)
subprocess.run(["systemctl","daemon-reload"],check=True)
for operation in jobs:
 subprocess.run(["systemctl","enable","--now","datacenter-host-"+operation+".timer"],check=True)
print("Independent protection jobs installed.")
