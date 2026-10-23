"""Private, sealed backup publication and conservative bounded retention."""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil

STAMP = re.compile(r"^[0-9]{8}T[0-9]{6}\.[0-9]{6}Z$")
MARKER = "backup-complete.json"

def sync_directory(path):
    fd = os.open(path, os.O_DIRECTORY)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)

def inventory(directory):
    result = {}
    for path in sorted(Path(directory).rglob("*")):
        if path.is_symlink():
            raise ValueError("Backup contains an unsupported link")
        if path.is_dir():
            continue
        if not path.is_file():
            raise ValueError("Backup contains an unsupported file")
        name = path.relative_to(directory).as_posix()
        if name == MARKER:
            continue
        digest = hashlib.sha256()
        with path.open("rb") as stream:
            for block in iter(lambda: stream.read(1024 * 1024), b""):
                digest.update(block)
            os.fsync(stream.fileno())
        result[name] = {"bytes": path.stat().st_size, "sha256": digest.hexdigest()}
    if not result or result.get("database.dump", {}).get("bytes", 0) == 0:
        raise ValueError("Backup database is missing or empty")
    return result

def seal(pending, target):
    pending, target = Path(pending), Path(target)
    if pending.parent != target.parent or not STAMP.fullmatch(target.name) or target.exists():
        raise ValueError("Invalid backup publication")
    data = {"format": "datacenter-backup-v1", "files": inventory(pending)}
    with (pending / MARKER).open("x") as stream:
        os.chmod(stream.name, 0o600)
        json.dump(data, stream, sort_keys=True)
        stream.flush()
        os.fsync(stream.fileno())
    for directory in sorted((p for p in pending.rglob("*") if p.is_dir()), reverse=True):
        sync_directory(directory)
    sync_directory(pending)
    os.rename(pending, target)
    sync_directory(target.parent)
    return target

def verified(path):
    path = Path(path)
    if path.is_symlink() or not path.is_dir() or not STAMP.fullmatch(path.name):
        return False
    marker = path / MARKER
    if marker.is_symlink() or not marker.is_file():
        return False
    try:
        data = json.loads(marker.read_text())
        return data.get("format") == "datacenter-backup-v1" and data.get("files") == inventory(path)
    except (OSError, ValueError):
        return False

def prune(root, keep=3):
    """Never delete legacy, incomplete, unrecognized or damaged backups."""
    if keep < 2:
        raise ValueError("At least two verified backups must be retained")
    root = Path(root)
    if root.is_symlink():
        raise ValueError("Backup root cannot be a link")
    candidates = sorted((p for p in root.iterdir() if STAMP.fullmatch(p.name)), reverse=True)
    valid = [p for p in candidates if verified(p)]
    removed = []
    for old in valid[keep:]:
        shutil.rmtree(old)
        removed.append(old.name)
    sync_directory(root)
    return removed
