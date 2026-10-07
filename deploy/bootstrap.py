#!/usr/bin/env python3
"""Publish and validate bootstrap credentials before the installer touches SQL."""
import base64
import os
from pathlib import Path
import re
import secrets
from urllib.parse import urlsplit, unquote
import sys
import tempfile

IDENT = re.compile(r"[A-Za-z_][A-Za-z0-9_]*\Z")


def atomic_write(path, data, mode, gid):
    fd, temp = tempfile.mkstemp(prefix="." + path.name + ".", dir=path.parent)
    try:
        with os.fdopen(fd, "wb") as stream:
            os.fchown(stream.fileno(), os.geteuid(), gid)
            os.fchmod(stream.fileno(), mode)
            stream.write(data)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temp, path)
        directory = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    finally:
        if os.path.exists(temp):
            os.unlink(temp)


def read_environment(env):
    values = {}
    for line in Path(env).read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if "=" not in line:
            raise ValueError("Malformed persisted environment")
        name, value = line.split("=", 1)
        # Supported EnvironmentFile subset: literal single-line assignments.
        # Never shell expansion, export, escapes, or inline comments.
        if value.startswith(('"', "'")) and value.endswith(value[0]):
            value = value[1:-1]
        if (not IDENT.fullmatch(name) or name in values or
                any(c.isspace() or c in "$\x60;()<>|&\\\"'\x00" for c in value)):
            raise ValueError("Unsupported persisted environment assignment")
        values[name] = value
    if any(not values.get(k) for k in ("DATABASE_URL", "MASTER_KEY_FILE", "MASTER_KEY_VERSION", "HTTP_ADDR")):
        raise ValueError("Incomplete persisted environment")
    url = urlsplit(values["DATABASE_URL"])
    if (url.scheme not in ("postgres", "postgresql") or not url.hostname or not url.password or
            not IDENT.fullmatch(unquote(url.username or "")) or not IDENT.fullmatch(unquote(url.path.lstrip("/"))) or url.fragment):
        raise ValueError("Invalid persisted database connection")
    return values

def prepare(directory, database, user, gid, validate_only=False):
    root = Path(directory)
    key, env, pending = (root / name for name in ("master.key", "env", "bootstrap.pending"))
    if validate_only and (not key.is_file() or not env.is_file() or pending.exists()):
        raise ValueError("Complete existing bootstrap required before upgrade")
    # A persisted identity is authoritative, even before env has been published.
    if pending.exists():
        lines = pending.read_text().splitlines()
        if len(lines) != 2 or any(not IDENT.fullmatch(v) for v in lines):
            raise ValueError("Invalid persisted bootstrap identity; preserve it for recovery")
        database, user = lines
    elif not env.exists():
        if not IDENT.fullmatch(database) or not IDENT.fullmatch(user):
            raise ValueError("Invalid database identifier")
    if key.exists():
        try:
            value = base64.b64decode(key.read_bytes().strip(), validate=True)
        except (ValueError, TypeError):
            raise ValueError("Invalid persisted master key; no credentials replaced") from None
        if len(value) != 32:
            raise ValueError("Incomplete persisted master key; no credentials replaced")
    elif env.exists():
        raise ValueError("Existing environment has no master key; refusing key rotation")
    else:
        atomic_write(key, base64.b64encode(secrets.token_bytes(32)) + b"\n", 0o640, gid)
    if not env.exists():
        if not pending.exists():
            atomic_write(pending, (database + "\n" + user + "\n").encode(), 0o600, gid)
        password = secrets.token_hex(24)
        body = (f"DATABASE_URL=postgres://{user}:{password}@127.0.0.1:5432/{database}?sslmode=disable\n"
                f"MASTER_KEY_FILE={key}\nMASTER_KEY_VERSION=1\nHTTP_ADDR=127.0.0.1:18080\n")
        atomic_write(env, body.encode(), 0o640, gid)
    values = read_environment(env)
    if values["MASTER_KEY_FILE"] != str(key) or not values["MASTER_KEY_VERSION"].isdigit() or int(values["MASTER_KEY_VERSION"]) < 1:
        raise ValueError("Persisted master-key identity mismatch")
    if pending.exists():
        lines = env.read_text().splitlines()
        pattern = (r"DATABASE_URL=postgres://" + re.escape(user) +
                   r":[a-f0-9]{48}@127\.0\.0\.1:5432/" + re.escape(database) + r"\?sslmode=disable")
        if (len(lines) != 4 or not re.fullmatch(pattern, lines[0]) or
                lines[1:] != [f"MASTER_KEY_FILE={key}", "MASTER_KEY_VERSION=1", "HTTP_ADDR=127.0.0.1:18080"]):
            raise ValueError("Bootstrap environment does not match persisted identity")


if __name__ == "__main__":
    try:
        if sys.argv[1] == "--database-url":
            print(read_environment(Path(sys.argv[2]) / "env")["DATABASE_URL"])
            raise SystemExit(0)
        prepare(sys.argv[1], sys.argv[2], sys.argv[3], int(sys.argv[4]), "--validate-only" in sys.argv[5:])
    except (ValueError, OSError) as exc:
        raise SystemExit(str(exc)) from None
