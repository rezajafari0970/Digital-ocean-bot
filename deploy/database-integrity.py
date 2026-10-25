#!/usr/bin/env python3
"""Read-only integrity inspection. Never repairs, resets statistics or restores data."""
import argparse
import datetime
import fcntl
import json
import os
from pathlib import Path
import runpy
import subprocess
import sys
import tempfile
from backup_store import seal, prune, verified, STAMP

ROOT = Path(__file__).resolve().parent
STATE = Path("/var/lib/datacenter-bot-health")
REPORT = STATE / "integrity.json"
SQL = """SELECT json_build_object(
 'database',current_database(),'oid',datid::text,
 'failures',COALESCE(checksum_failures,0),
 'last_failure',COALESCE(extract(epoch FROM checksum_last_failure),0)::float8,
 'stats_reset',COALESCE(extract(epoch FROM stats_reset),0)::float8,
 'checksums',current_setting('data_checksums'),
 'fsync',current_setting('fsync'),
 'full_page_writes',current_setting('full_page_writes'),
 'synchronous_commit',current_setting('synchronous_commit'),
 'zero_damaged_pages',current_setting('zero_damaged_pages'),
 'ignore_checksum_failure',current_setting('ignore_checksum_failure')
) FROM pg_stat_database WHERE datname=current_database()"""

def now():
    return datetime.datetime.now(datetime.timezone.utc)

def query(env, sql):
    p = subprocess.run(["psql", "-XqAt", "-v", "ON_ERROR_STOP=1"], input=sql,
                       env=env, text=True, capture_output=True, timeout=30)
    if p.returncode:
        raise RuntimeError("database_unavailable")
    return p.stdout.strip()

def signature(stats):
    return {k: stats[k] for k in ("database", "oid", "failures", "last_failure", "stats_reset")}

def needs_acknowledgement(previous, stats):
    if not previous or (previous.get("watermark") is None and not previous.get("recovery_required")):
        return stats["failures"] != 0 or stats["last_failure"] != 0
    return previous.get("recovery_required", False) or previous.get("watermark") != signature(stats)

def publish(data):
    fd, name = tempfile.mkstemp(prefix=".integrity-", dir=STATE)
    try:
        with os.fdopen(fd, "w") as stream:
            os.fchmod(stream.fileno(), 0o644)
            json.dump(data, stream, sort_keys=True)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(name, REPORT)
        fd = os.open(STATE, os.O_DIRECTORY)
        try:
            os.fsync(fd)
        finally:
            os.close(fd)
    finally:
        if os.path.exists(name):
            os.unlink(name)

def validate_settings(stats):
    safe = {"checksums": "on", "fsync": "on", "full_page_writes": "on",
            "zero_damaged_pages": "off", "ignore_checksum_failure": "off"}
    if any(stats.get(k) != v for k, v in safe.items()) or stats.get("synchronous_commit") not in ("on", "local", "remote_write", "remote_apply"):
        raise RuntimeError("unsafe_database_settings")

def inspect(env, previous, acknowledge=False):
    stats = json.loads(query(env, SQL))
    required = needs_acknowledgement(previous, stats)
    validate_settings(stats)
    version = int(query(env, "SHOW server_version_num")) // 10000
    binary = Path("/usr/lib/postgresql") / str(version) / "bin/pg_amcheck"
    if not binary.is_file():
        raise RuntimeError("integrity_checker_unavailable")
    # The installed application owns a local database. Peer authentication avoids
    # handing the application credential superuser privileges.
    if env.get("PGHOST") not in ("127.0.0.1", "localhost", "/var/run/postgresql"):
        raise RuntimeError("local_database_required")
    args = ["/usr/sbin/runuser", "-u", "postgres", "--", str(binary),
            "-h", "/var/run/postgresql", "-p", env.get("PGPORT", "5432"),
            "--install-missing", "--jobs=1", env["PGDATABASE"]]
    # Root-only diagnostics are replaced each run rather than growing forever.
    log = STATE / "last-check.log"
    with log.open("w") as output:
        os.chmod(log, 0o600)
        p = subprocess.run(args, stdout=output, stderr=subprocess.STDOUT, timeout=1800)
    if p.returncode:
        raise RuntimeError("integrity_check_failed")
    after = json.loads(query(env, SQL))
    validate_settings(after)
    if signature(stats) != signature(after):
        raise RuntimeError("database_integrity_changed")
    if required and not acknowledge:
        raise RuntimeError("verified_recovery_acknowledgement_required")
    return {"version": 1, "status": "ok", "checked_at": now().timestamp(),
            "watermark": signature(after), "checksums": after["checksums"], "recovery_required": False}

def has_recent_verified_backup(root, current):
    # Use the sealed generation timestamp, never mutable directory metadata or
    # the timestamp of a different (possibly damaged) generation.
    for path in sorted(root.iterdir(), reverse=True):
        if path.is_symlink() or not path.is_dir() or not STAMP.fullmatch(path.name):
            continue
        try:
            created = datetime.datetime.strptime(path.name, "%Y%m%dT%H%M%S.%fZ").replace(tzinfo=datetime.timezone.utc)
        except ValueError:
            continue
        age = (current - created).total_seconds()
        if 0 <= age < 20 * 3600 and verified(path):
            return True
    return False

def daily_backup(env):
    root = Path("/var/backups/datacenter-bot-daily")
    root.mkdir(mode=0o700, parents=True, exist_ok=True)
    os.chmod(root, 0o700)
    if has_recent_verified_backup(root, now()):
        return
    free = os.statvfs(root)
    size = int(query(env, "SELECT pg_database_size(current_database())"))
    if free.f_bavail * free.f_frsize < max(5 * 1024**3, size * 1.1 + 2 * 1024**3):
        raise RuntimeError("backup_disk_space_low")
    stamp = now().strftime("%Y%m%dT%H%M%S.%fZ")
    pending = Path(tempfile.mkdtemp(prefix=".pending-", dir=root))
    # A failed dump remains visibly incomplete and can never displace a good backup.
    with (pending / "database.dump").open("wb") as output:
        p = subprocess.run(["pg_dump", "--format=custom", "--no-owner"], env=env,
                           stdout=output, stderr=subprocess.DEVNULL, timeout=1800)
    if p.returncode:
        raise RuntimeError("database_backup_failed")
    subprocess.run(["pg_restore", "--file=/dev/null", str(pending / "database.dump")],
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True, timeout=1800)
    # Encryption identity is necessary to use a database backup after host loss.
    import shutil
    shutil.copytree("/etc/digital-ocean-bot", pending / "configuration", symlinks=True)
    seal(pending, root / stamp)
    prune(root, keep=7)

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--acknowledge-repair", action="store_true",
                        help="Only after independent verification of a completed database repair")
    parser.add_argument("--backup", action="store_true")
    args = parser.parse_args()
    if os.geteuid() != 0:
        raise SystemExit("Run with sudo.")
    os.umask(0o077)
    # The installer holds this lock across configuration/schema publication.
    if args.backup:
        install_lock = open("/run/lock/datacenter-bot-installer.lock", "a")
        try:
            fcntl.flock(install_lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            print("Deployment in progress; scheduled inspection deferred.")
            return
    STATE.mkdir(mode=0o755, exist_ok=True)
    os.chmod(STATE, 0o755)
    with (STATE / "check.lock").open("w") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise SystemExit("Database integrity check is already running.")
        previous = None
        if REPORT.exists():
            try:
                previous = json.loads(REPORT.read_text())
            except (ValueError, OSError):
                previous = {"recovery_required": True}
        helpers = runpy.run_path(str(ROOT / "bootstrap.py"))
        env = helpers["database_environment"](helpers["read_environment"](Path("/etc/digital-ocean-bot/env")))
        try:
            result = inspect(env, previous, args.acknowledge_repair)
        except Exception as exc:
            reason = str(exc) if isinstance(exc, RuntimeError) else "integrity_check_incomplete"
            # A failed/interrupted inspection cannot erase a prior corruption alarm.
            publish({"version": 1, "status": "failed", "checked_at": now().timestamp(),
                     "reason": reason, "watermark": (previous or {}).get("watermark"),
                     "recovery_required": reason in ("integrity_check_failed", "database_integrity_changed", "verified_recovery_acknowledgement_required") or (previous or {}).get("recovery_required", False)})
            raise SystemExit("Database integrity needs attention: " + reason)
        result["backup"] = (previous or {}).get("backup")
        publish(result)
        print("Database integrity inspection passed; no data was repaired or removed.")
        if args.backup:
            try:
                daily_backup(env)
                result["backup"] = {"ok": True, "checked_at": now().timestamp()}
                publish(result)
            except Exception:
                result["backup"] = {"ok": False, "checked_at": now().timestamp()}
                publish(result)
                raise SystemExit("Database backup failed; existing backups were preserved.")

if __name__ == "__main__":
    main()
