#!/usr/bin/env python3
"""Compress old production runtime copies; preserve every byte and three hot rollbacks.

Only the production adapter's exact dated rollback layout is supported. No
database, settings, backups, source worktrees or another project's path is eligible.
"""
import argparse
import contextlib
import datetime as dt
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import stat
import tarfile
import tempfile
import time

NAME = re.compile(r"dob-production\.rollback-[0-9]{8}T[0-9]{6}\Z")
PENDING = ".archive-pending"
STATE = Path("/var/lib/dob-resource-maintenance")

def digest(path):
    h = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()

def syncdir(path):
    fd = os.open(path, os.O_RDONLY | os.O_DIRECTORY)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)

def atomic(path, value):
    fd, tmp = tempfile.mkstemp(dir=path.parent, prefix=".receipt-")
    try:
        with os.fdopen(fd, "w") as stream:
            json.dump(value, stream, sort_keys=True)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(tmp, path)
        syncdir(path.parent)
    finally:
        if os.path.exists(tmp):
            os.unlink(tmp)

def inventory(path):
    if path.is_symlink() or not path.is_dir():
        raise ValueError("runtime_directory_required")
    out = {}
    for child in sorted([path] + list(path.rglob("*"))):
        info = child.lstat()
        if not (stat.S_ISDIR(info.st_mode) or stat.S_ISREG(info.st_mode)):
            raise ValueError("runtime_links_or_special_files_refused")
        out[str(child.relative_to(path))] = {
            "mode": stat.S_IMODE(info.st_mode), "uid": info.st_uid, "gid": info.st_gid,
            "kind": "dir" if child.is_dir() else "file",
            "size": info.st_size if child.is_file() else 0,
            "sha256": digest(child) if child.is_file() else "",
            "mtime_ns": info.st_mtime_ns,
        }
    return out

def verify_runtime(path):
    data = json.loads((path / "build-manifest.json").read_text())
    if not re.fullmatch(r"[0-9a-f]{40}", data["commit"]):
        raise ValueError("invalid_runtime_revision")
    for filename, key in [("digital-ocean-bot-api", "api_sha256"),
                          ("digital-ocean-bot-worker", "worker_sha256")]:
        if digest(path / "bin" / filename) != data[key]:
            raise ValueError("runtime_identity_mismatch")
    return data["commit"]

def verify_archive(path, expected):
    seen = set()
    with tarfile.open(path, "r:gz") as archive:
        for member in archive:
            name = member.name
            if name in seen or name not in expected:
                raise ValueError("archive_member_mismatch")
            seen.add(name)
            item = expected[name]
            if (member.mode != item["mode"] or member.uid != item["uid"] or
                    member.gid != item["gid"]):
                raise ValueError("archive_metadata_mismatch")
            if item["kind"] == "dir":
                if not member.isdir():
                    raise ValueError("archive_directory_mismatch")
            else:
                if not member.isfile() or member.size != item["size"]:
                    raise ValueError("archive_file_mismatch")
                stream = archive.extractfile(member)
                h = hashlib.sha256()
                for block in iter(lambda: stream.read(1024 * 1024), b""):
                    h.update(block)
                if h.hexdigest() != item["sha256"]:
                    raise ValueError("archive_content_mismatch")
    if seen != set(expected):
        raise ValueError("archive_incomplete")

def references(path):
    """Conservatively refuse a runtime held by any process; never print arguments."""
    prefix = str(path) + "/"
    for proc in Path("/proc").iterdir():
        if not proc.name.isdigit():
            continue
        try:
            links = [proc / n for n in ("cwd", "exe", "root")]
            links.extend((proc / "fd").iterdir())
            for link in links:
                try:
                    target = os.readlink(link).removesuffix(" (deleted)")
                except FileNotFoundError:
                    continue
                if target == str(path) or target.startswith(prefix):
                    return True
            maps = (proc / "maps").read_text()
            if prefix in maps:
                return True
        except (FileNotFoundError, ProcessLookupError):
            continue
        except PermissionError:
            raise ValueError("process_ownership_unverified") from None
    return False

def archive_one(path, destination, in_use=references):
    name = path.name.removesuffix(PENDING)
    if not NAME.fullmatch(name) or path.is_symlink():
        raise ValueError("unowned_runtime")
    destination.mkdir(mode=0o700, parents=True, exist_ok=True)
    if destination.is_symlink():
        raise ValueError("archive_directory_symlink")
    target = destination / (name + ".tar.gz")
    receipt = path.parent / (name + ".archived.json")
    pending = path.parent / (name + PENDING)
    if path == pending:
        saved = json.loads(receipt.read_text())
        if (saved["source"] != str(path.parent / name) or saved["archive"] != str(target)
                or not target.is_file() or target.is_symlink()
                or digest(target) != saved["archive_sha256"]):
            raise ValueError("pending_archive_unverified")
        verify_archive(target, saved["files"])
        remaining = inventory(pending)
        # Directory mtimes may change during an interrupted cleanup.
        for key, item in remaining.items():
            old = saved["files"].get(key)
            if not old or any(item[k] != old[k] for k in ("kind", "mode", "uid", "gid", "size", "sha256")):
                raise ValueError("pending_runtime_changed")
        if in_use(pending):
            raise ValueError("runtime_in_use")
        shutil.rmtree(pending)
        syncdir(path.parent)
        return saved
    if pending.exists() or in_use(path):
        raise ValueError("runtime_in_use_or_pending")
    before = inventory(path)
    commit = verify_runtime(path)
    fd, temporary = tempfile.mkstemp(prefix="." + name + "-", dir=destination)
    os.close(fd)
    temporary = Path(temporary)
    try:
        with tarfile.open(temporary, "w:gz", compresslevel=1, format=tarfile.PAX_FORMAT) as archive:
            for item in before:
                archive.add(path if item == "." else path / item, arcname=item, recursive=False)
        verify_archive(temporary, before)
        with temporary.open("rb") as stream:
            os.fsync(stream.fileno())
        if inventory(path) != before or in_use(path):
            raise ValueError("runtime_changed")
        os.replace(temporary, target)
        syncdir(destination)
        saved = {"format": 1, "source": str(path), "archive": str(target),
                 "archive_sha256": digest(target), "commit": commit, "files": before,
                 "source_bytes": sum(x["size"] for x in before.values()),
                 "archive_bytes": target.stat().st_size, "at": time.time()}
        atomic(receipt, saved)
        os.rename(path, pending)
        syncdir(path.parent)
        # Resume uses the same verified receipt after an interruption.
        return archive_one(pending, destination, in_use)
    finally:
        temporary.unlink(missing_ok=True)

def candidates(root, now):
    paths = [p for p in root.iterdir() if NAME.fullmatch(p.name) and p.is_dir() and not p.is_symlink()]
    paths.sort(key=lambda p: p.name, reverse=True)
    result = []
    for path in paths[3:]:
        stamp = dt.datetime.strptime(path.name.split("rollback-")[1], "%Y%m%dT%H%M%S").replace(tzinfo=dt.timezone.utc).timestamp()
        if now - stamp >= 7 * 86400:
            result.append(path)
    return sorted(result, key=lambda p: p.name)

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--apply", action="store_true")
    args = parser.parse_args()
    if os.geteuid() != 0:
        raise SystemExit("root_required")
    os.umask(0o077)
    root = Path("/opt")
    STATE.mkdir(mode=0o700, parents=True, exist_ok=True)
    with contextlib.ExitStack() as stack:
        # Same lock ordering as the production installer. Skip a busy deployment.
        for name in ["/run/lock/dob-production-installer.lock", "/opt/.dob-production-deploy.lock"]:
            fd = os.open(name, os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
            stack.callback(os.close, fd)
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        verify_runtime(root / "dob-production")
        if Path("/etc/dob-production/deployment.pending").exists():
            raise ValueError("deployment_pending")
        pending = [p for p in root.iterdir() if p.name.endswith(PENDING) and NAME.fullmatch(p.name.removesuffix(PENDING))]
        selected = pending + candidates(root, time.time())
        report = {"at": time.time(), "eligible": len(selected), "applied": [], "errors": []}
        for path in selected[:2]:
            if not args.apply:
                continue
            try:
                row = archive_one(path, STATE / "archives")
                report["applied"].append({k: row[k] for k in ("source", "archive", "source_bytes", "archive_bytes", "commit")})
            except (ValueError, OSError, KeyError, tarfile.TarError) as exc:
                report["errors"].append({"source": str(path), "type": type(exc).__name__})
        atomic(STATE / "rollback-archive-status.json", report)
        print(json.dumps(report))
        if report["errors"]:
            raise SystemExit(1)

if __name__ == "__main__":
    main()
