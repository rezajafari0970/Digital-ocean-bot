#!/usr/bin/env python3
"""Refuse disk-starved deployment before any runtime readers are stopped."""
import argparse
import os
from pathlib import Path
import runpy
import subprocess

GIB = 1024 ** 3

def existing_parent(path):
    path = Path(path)
    while not path.exists():
        if path.parent == path:
            raise RuntimeError("Cannot locate deployment filesystem")
        path = path.parent
    return path

def check_capacity(requirements, *, stat=os.stat, statvfs=os.statvfs):
    """Add simultaneous reserves on the same filesystem, not per pathname."""
    devices = {}
    for path, reserve, inodes in requirements:
        path = existing_parent(path)
        device = stat(path).st_dev
        entry = devices.setdefault(device, [path, 0, 0])
        entry[1] += reserve
        entry[2] += inodes
    for path, reserve, inodes in devices.values():
        fs = statvfs(path)
        free = fs.f_bavail * fs.f_frsize
        if free < reserve or (fs.f_files and fs.f_favail < inodes):
            raise RuntimeError(
                f"Insufficient deployment space on {path}: "
                f"{free // (1024**2)} MiB available, {reserve // (1024**2)} MiB required; "
                "running services have not been stopped. Reclaim reproducible cache or add storage."
            )

def backup_size(etc):
    envfile = Path(etc) / "env"
    if not envfile.exists():
        return 0
    helpers = runpy.run_path(str(Path(__file__).with_name("bootstrap.py")))
    env = helpers["database_environment"](helpers["read_environment"](envfile))
    result = subprocess.run(
        ["psql", "-XqAt", "-v", "ON_ERROR_STOP=1", "-c",
         "SELECT pg_database_size(current_database())"],
        env=env, capture_output=True, text=True, timeout=15,
    )
    if result.returncode or not result.stdout.strip().isdigit():
        raise RuntimeError("Cannot verify database backup size before deployment; services remain running")
    return int(result.stdout.strip())

def tree_backup_bytes(root):
    total = 0
    def fail(error):
        raise error
    for directory, dirs, files in os.walk(root, followlinks=False, onerror=fail):
        for name in dirs + files:
            info = os.lstat(Path(directory) / name)
            # Include tar headers and block padding; symlinks are archived as links.
            total += ((info.st_size + 511) // 512 + 4) * 512
    return total


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("phase", choices=["build", "publish"])
    parser.add_argument("--etc", default="/etc/digital-ocean-bot")
    args = parser.parse_args()
    if args.phase == "build":
        requirements = [("/opt", 3 * GIB, 50000)]
    else:
        size = backup_size(args.etc)
        persistent = tree_backup_bytes("/var/lib/digital-ocean-bot") + tree_backup_bytes(args.etc)
        # A full dump plus conservative expansion, new release metadata and DB
        # write headroom. No historical backups or user data are deleted.
        requirements = [
            ("/var/backups/datacenter-bot", 2 * size + persistent + GIB, 5000),
            ("/opt", GIB // 2, 5000),
        ]
        if Path("/var/lib/postgresql").exists():
            requirements.append(("/var/lib/postgresql", GIB, 10000))
    check_capacity(requirements)
    print(f"Deployment disk reserve verified before {args.phase}")

if __name__ == "__main__":
    try:
        main()
    except (OSError, RuntimeError, subprocess.SubprocessError) as exc:
        raise SystemExit(str(exc))
