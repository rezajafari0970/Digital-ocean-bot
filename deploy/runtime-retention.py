#!/usr/bin/env python3
"""Retire only marked, verified runtime rollback copies after a successful release."""
import hashlib
import json
from pathlib import Path
import re
import shutil
import sys
import time

MARKER = "release-complete.json"

def digest(path):
    if path.is_symlink() or not path.is_file():
        raise ValueError("Invalid runtime artifact")
    with path.open("rb") as stream:
        digest = hashlib.sha256()
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
        return digest.hexdigest()

def identity(path):
    if path.is_symlink() or not path.is_dir():
        raise ValueError("Invalid rollback directory")
    manifest = path / "runtime/build-manifest.json"
    data = json.loads(manifest.read_text())
    if not re.fullmatch("[a-f0-9]{40}", data["commit"]):
        raise ValueError("Invalid rollback revision")
    for file, key in (("digital-ocean-bot-api", "api_sha256"), ("digital-ocean-bot-worker", "worker_sha256")):
        if digest(path / "runtime/bin" / file) != data[key]:
            raise ValueError("Rollback binary hash mismatch")
    return digest(manifest)

def retain(current, root=Path("/opt")):
    current = Path(current)
    root = Path(root)
    if current.parent != root or not re.fullmatch(r"digital-ocean-bot\.rollback\.[A-Za-z0-9]+", current.name):
        raise ValueError("Invalid rollback target")
    # Fresh installs have no preceding runtime.
    if not (current / "runtime/build-manifest.json").exists():
        return []
    signature = identity(current)
    marker = current / MARKER
    with marker.open("x") as output:
        json.dump({"format": 1, "manifest": signature, "completed": time.time()}, output)
    candidates = []
    for path in root.glob("digital-ocean-bot.rollback.*"):
        try:
            mark = path / MARKER
            if mark.is_symlink():
                continue
            saved = json.loads(mark.read_text())
            if saved["format"] == 1 and saved["manifest"] == identity(path):
                candidates.append((saved["completed"], path))
        except (OSError, ValueError, KeyError, TypeError):
            continue
    removed = []
    for _, path in sorted(candidates, reverse=True)[2:]:
        shutil.rmtree(path)
        removed.append(path.name)
    return removed

if __name__ == "__main__":
    print("Expired verified runtime copies removed: " + str(len(retain(sys.argv[1]))))
