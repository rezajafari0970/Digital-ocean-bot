#!/usr/bin/env python3
"""Independent host evidence and encrypted off-host backup jobs; never repairs data."""
import argparse
import datetime as dt
import fcntl
import hashlib
import json
import os
from pathlib import Path
import runpy
import socket
import subprocess
import tempfile
import time

CONFIG = Path("/etc/datacenter-host-protection")
STATE = Path("/var/lib/datacenter-host-protection")
EVIDENCE = STATE / "evidence"

def atomic_json(path, value, mode=0o600):
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, temp = tempfile.mkstemp(prefix=".pending-", dir=path.parent)
    try:
        with os.fdopen(fd, "w") as f:
            os.fchmod(f.fileno(), mode)
            json.dump(value, f, sort_keys=True)
            f.flush()
            os.fsync(f.fileno())
        os.replace(temp, path)
        fd = os.open(path.parent, os.O_DIRECTORY)
        try:
            os.fsync(fd)
        finally:
            os.close(fd)
    finally:
        if os.path.exists(temp):
            os.unlink(temp)

def app_source():
    path = Path("/usr/local/lib/datacenter-bot/source").read_text().strip()
    return Path(path) / "deploy"

def db_stats():
    helpers = runpy.run_path(str(app_source() / "bootstrap.py"))
    env = helpers["database_environment"](helpers["read_environment"](Path("/etc/digital-ocean-bot/env")))
    env["PGOPTIONS"] = "-c default_transaction_read_only=on -c statement_timeout=10000"
    sql = """SELECT json_build_object('database',current_database(),'oid',datid::text,
    'failures',COALESCE(checksum_failures,0),
    'last_failure',COALESCE(extract(epoch FROM checksum_last_failure),0)::float8,
    'stats_reset',COALESCE(extract(epoch FROM stats_reset),0)::float8)
    FROM pg_stat_database WHERE datname=current_database()"""
    result = subprocess.run(["psql", "-XqAt", "-v", "ON_ERROR_STOP=1"], input=sql,
                            env=env, capture_output=True, text=True, timeout=15)
    if result.returncode:
        raise RuntimeError("database_evidence_unavailable")
    return json.loads(result.stdout)

def eligible_backup(backup, report, stats, now):
    if report.get("status") != "ok" or report.get("recovery_required"):
        raise RuntimeError("database_recovery_required")
    if not 0 <= now - report.get("checked_at", 0) < 8 * 3600:
        raise RuntimeError("integrity_report_stale")
    if report.get("watermark") != stats:
        raise RuntimeError("database_integrity_changed")
    if not 0 <= now - backup.stat().st_mtime < 30 * 3600:
        raise RuntimeError("local_backup_stale")

def restic_env(vault=False):
    env = os.environ.copy()
    if vault:
        env["RESTIC_REPOSITORY"] = "/var/lib/datacenter-backup-vault/primary"
        credentials = os.environ.get("CREDENTIALS_DIRECTORY")
        if not credentials:
            raise RuntimeError("vault_service_credentials_required")
        env.pop("RESTIC_PASSWORD", None)
        env["RESTIC_PASSWORD_FILE"] = str(Path(credentials) / "restic-password")
    else:
        for line in (CONFIG / "client.env").read_text().splitlines():
            if line and not line.startswith("#"):
                key, value = line.split("=", 1)
                env[key] = value
    return env

def restic(args, vault=False, timeout=1800):
    command = (["/usr/bin/restic", "--json", "--no-cache"] if vault else ["/usr/bin/restic", "--json", "--cache-dir", str(STATE / "restic-cache")])
    if not vault:
        command += ["--cacert", str(CONFIG / "vault.crt")]
    result = subprocess.run(command + args, env=restic_env(vault),
                            capture_output=True, text=True, timeout=timeout)
    if result.returncode:
        # Never log repository credentials or raw remote responses.
        raise RuntimeError("restic_operation_failed_" + str(result.returncode))
    return result.stdout

def evidence():
    EVIDENCE.mkdir(mode=0o700, parents=True, exist_ok=True)
    boot = Path("/proc/sys/kernel/random/boot_id").read_text().strip()
    kernel = EVIDENCE / ("kernel-" + boot + ".log")
    result = subprocess.run(["dmesg", "--time-format", "iso"], capture_output=True,
                            text=True, timeout=15)
    if result.returncode == 0:
        pending = kernel.with_suffix(".pending")
        pending.write_text(result.stdout)
        os.replace(pending, kernel)
    durable_kernel = Path("/var/log/datacenter-storage-kernel.log")
    if durable_kernel.is_file():
        stamp = dt.datetime.now(dt.timezone.utc).strftime("%Y%m%d")
        (EVIDENCE / ("kernel-events-" + stamp + ".log")).write_bytes(durable_kernel.read_bytes()[-5 * 1024 * 1024:])
    value = {"time": time.time(), "boot": boot, "host": socket.gethostname(),
             "kernel": os.uname().release, "free_bytes": os.statvfs("/var/lib/postgresql").f_bavail * os.statvfs("/var/lib/postgresql").f_frsize}
    for base, pattern in [("/sys/fs/ext4", "*/errors_count"), ("/sys/block", "*/queue/write_cache")]:
        for path in Path(base).glob(pattern):
            value[str(path)] = path.read_text().strip()
    try:
        value["database"] = db_stats()
    except Exception:
        value["database_error"] = "database_evidence_unavailable"
    report = Path("/var/lib/datacenter-bot-health/integrity.json")
    if report.exists():
        value["integrity"] = json.loads(report.read_text())
        if value["integrity"].get("status") != "ok":
            log = Path("/var/lib/datacenter-bot-health/last-check.log")
            if log.exists():
                raw = log.read_bytes()[:1024 * 1024]
                digest = hashlib.sha256(raw).hexdigest()
                (EVIDENCE / ("integrity-failure-" + digest + ".log")).write_bytes(raw)
    stamp = dt.datetime.now(dt.timezone.utc).strftime("%Y%m%d")
    with (EVIDENCE / ("storage-" + stamp + ".jsonl")).open("a") as stream:
        stream.write(json.dumps(value, sort_keys=True) + "\n")
        stream.flush()
        os.fsync(stream.fileno())
    # Keep evidence independently of high-volume application logging.
    for path in EVIDENCE.iterdir():
        if path.is_file() and not path.is_symlink() and path.name.startswith(("kernel-", "storage-", "integrity-failure-")):
            if time.time() - path.stat().st_mtime > 90 * 86400:
                path.unlink()
    atomic_json(STATE / "evidence-status.json", {"ok": True, "checked_at": time.time(), "database_available": "database" in value})

def backup():
    previous = {}
    status_path = STATE / "offhost-status.json"
    if status_path.exists():
        previous = json.loads(status_path.read_text())
    result = {"ok": False, "checked_at": time.time(), "last_success": previous.get("last_success")}
    database_error = None
    try:
        # Evidence upload remains useful even when database validation is blocked.
        evidence()
        restic(["backup", "--host", socket.gethostname(), "--tag", "dob-evidence", str(EVIDENCE)])
        import sys
        sys.path.insert(0, str(app_source()))
        from backup_store import verified, STAMP
        paths = sorted((p for p in Path("/var/backups/datacenter-bot-daily").iterdir()
                        if STAMP.fullmatch(p.name) and not p.is_symlink()), reverse=True)
        selected = next((p for p in paths if verified(p)), None)
        if selected is None:
            raise RuntimeError("verified_local_backup_missing")
        report = json.loads(Path("/var/lib/datacenter-bot-health/integrity.json").read_text())
        eligible_backup(selected, report, db_stats(), time.time())
        digest = hashlib.sha256((selected / "backup-complete.json").read_bytes()).hexdigest()
        snapshots = json.loads(restic(["snapshots", "--host", socket.gethostname(), "--tag", "dob-database,seal:" + digest]))
        if not snapshots:
            restic(["backup", "--host", socket.gethostname(), "--tag", "dob-database",
                    "--tag", "seal:" + digest, str(selected)])
            snapshots = json.loads(restic(["snapshots", "--host", socket.gethostname(), "--tag", "dob-database,seal:" + digest]))
        if not snapshots:
            raise RuntimeError("remote_snapshot_not_confirmed")
        result.update(ok=True, snapshot=snapshots[-1]["id"], seal=digest, last_success=time.time())
    except Exception as exc:
        database_error = str(exc) if isinstance(exc, RuntimeError) else "offhost_backup_incomplete"
        result["reason"] = database_error
    atomic_json(status_path, result)
    if database_error:
        raise RuntimeError(database_error)
    print("Encrypted off-host snapshot confirmed.")

def retain():
    # Only the vault host has this path; clients cannot delete remote snapshots.
    restic(["check"], vault=True)
    for tag, daily, weekly in [("dob-database", "7", "2"), ("dob-evidence", "14", "0")]:
        snapshots = json.loads(restic(["snapshots", "--tag", tag], vault=True))
        if len(snapshots) < 3:
            continue
        restic(["forget", "--tag", tag, "--group-by", "host", "--keep-last", "2",
                "--keep-daily", daily, "--keep-weekly", weekly, "--prune"], vault=True)
    atomic_json(STATE / "vault-status.json", {"ok": True, "checked_at": time.time()})
    print("Vault integrity and retention completed.")

def main():
    global STATE
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("operation", choices=["evidence", "backup", "retain", "status"])
    args = parser.parse_args()
    if args.operation == "retain":
        import pwd
        if os.geteuid() != pwd.getpwnam("dob-backup-vault").pw_uid:
            raise SystemExit("Run systemctl start datacenter-host-retain.service.")
        STATE = Path("/var/lib/datacenter-backup-maintenance")
    elif os.geteuid() != 0:
        raise SystemExit("Run with sudo.")
    os.umask(0o077)
    STATE.mkdir(mode=0o700, parents=True, exist_ok=True)
    if args.operation == "status":
        for name in ["evidence-status.json", "offhost-status.json", "vault-status.json"]:
            path = (Path("/var/lib/datacenter-backup-maintenance") if name == "vault-status.json" else STATE) / name
            if path.exists():
                value = json.loads(path.read_text())
                limit = {"evidence-status.json": 1200, "offhost-status.json": 21600, "vault-status.json": 172800}[name]
                value["fresh"] = 0 <= time.time() - value.get("checked_at", 0) < limit
                print(name, json.dumps(value, sort_keys=True))
        return
    with (STATE / (args.operation + ".lock")).open("w") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            print("Independent protection job already running.")
            return
        try:
            {"evidence": evidence, "backup": backup, "retain": retain}[args.operation]()
        except Exception as exc:
            reason = str(exc) if isinstance(exc, RuntimeError) else "protection_job_incomplete"
            if args.operation == "retain":
                atomic_json(STATE / "vault-status.json", {"ok": False, "checked_at": time.time(), "reason": reason})
            print("Host protection needs attention: " + reason)
            raise SystemExit(1)

if __name__ == "__main__":
    main()
