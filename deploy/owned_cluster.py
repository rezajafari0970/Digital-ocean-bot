"""Create a checksummed cluster only for a new, durably identified installation."""
import json
import re
import subprocess
from pathlib import Path
import secrets

def plan_cluster(binary_root=Path("/usr/lib/postgresql")):
    versions = [int(p.name) for p in binary_root.iterdir()
                if p.name.isdigit() and 14 <= int(p.name) <= 18 and (p / "bin/initdb").is_file()]
    if not versions:
        raise RuntimeError("Supported PostgreSQL server binaries are unavailable")
    return {"version": max(versions), "name": "dob" + secrets.token_hex(8), "port": 55432}

def ensure_cluster(state, save, *, run=subprocess.run, config_root=Path("/etc/postgresql"),
                   data_root=Path("/var/lib/postgresql")):
    cluster = state.get("cluster")
    if not isinstance(cluster, dict):
        raise RuntimeError("Missing persisted cluster intent")
    version, name, port = cluster.get("version"), cluster.get("name"), cluster.get("port")
    if type(version) is not int or not 14 <= version <= 18 or not isinstance(name, str) or not re.fullmatch("dob[a-f0-9]{16}", name) or port != 55432:
        raise RuntimeError("Invalid persisted cluster identity")
    config = config_root / str(version) / name
    data = data_root / str(version) / name
    if config.is_symlink() or data.is_symlink():
        raise RuntimeError("Cluster paths cannot be links")
    if not config.exists():
        if data.exists() or cluster.get("system_identifier"):
            raise RuntimeError("Incomplete or missing cluster preserved for recovery")
        # The intent was persisted by the caller before this side effect.
        result = run(["pg_createcluster", str(version), name, "--port", str(port),
                      "--datadir", str(data), "--start-conf=auto", "--start",
                      "-o", "listen_addresses=127.0.0.1", "--",
                      "--data-checksums", "--auth-local=peer", "--auth-host=scram-sha-256"],
                     capture_output=True, text=True, timeout=120)
        if result.returncode:
            raise RuntimeError("Dedicated cluster creation incomplete; preserve intent and inspect before retry")
    result = run(["pg_ctlcluster", str(version), name, "status"], capture_output=True, text=True, timeout=15)
    if result.returncode:
        result = run(["pg_ctlcluster", str(version), name, "start"], capture_output=True, text=True, timeout=60)
        if result.returncode:
            raise RuntimeError("Owned cluster could not start")
    sql = """SELECT json_build_object(
        'system_identifier',(SELECT system_identifier::text FROM pg_control_system()),
        'checksums',current_setting('data_checksums'),
        'data_directory',current_setting('data_directory'),
        'port',current_setting('port'),
        'version',(current_setting('server_version_num')::int/10000),
        'foreign_databases',(SELECT count(*) FROM pg_database WHERE datname NOT IN ('postgres','template0','template1')),
        'foreign_roles',(SELECT count(*) FROM pg_roles WHERE rolname<>'postgres' AND rolname !~ '^pg_')
    )"""
    result = run(["runuser", "-u", "postgres", "--", "psql", "-h", "/var/run/postgresql",
                  "-p", str(port), "-d", "postgres", "-XAt", "-v", "ON_ERROR_STOP=1", "-c", sql],
                 capture_output=True, text=True, timeout=30)
    if result.returncode:
        raise RuntimeError("Dedicated cluster identity check failed")
    actual = json.loads(result.stdout)
    if (actual.get("checksums") != "on" or actual.get("data_directory") != str(data) or
            actual.get("port") != str(port) or actual.get("version") != version or
            not str(actual.get("system_identifier", "")).isdigit()):
        raise RuntimeError("Dedicated cluster identity or checksum mismatch")
    if cluster.get("system_identifier"):
        if cluster["system_identifier"] != actual["system_identifier"]:
            raise RuntimeError("Dedicated cluster was replaced; refusing adoption")
    else:
        if actual.get("foreign_databases") != 0 or actual.get("foreign_roles") != 0:
            raise RuntimeError("Unverified cluster contains existing user data")
        cluster["system_identifier"] = actual["system_identifier"]
        save(state)
    return port
