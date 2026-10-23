#!/usr/bin/env python3
"""Fixed-action root update bridge. HTTP authorization lives in the panel API."""
import contextlib
import datetime
import fcntl
import json
import os
import pathlib
import pwd
import re
import socket
import struct
import subprocess
import sys
import tempfile
import time
import uuid

STATE = pathlib.Path("/var/lib/datacenter-bot-updater")
RUNTIME = pathlib.Path("/run/datacenter-bot")
MANIFEST = pathlib.Path("/opt/digital-ocean-bot/build-manifest.json")
PENDING = pathlib.Path("/etc/digital-ocean-bot/deployment.pending")
INSTALLER_LOCK = "/run/lock/datacenter-bot-installer.lock"
UPDATE_UNIT = "datacenter-bot-update.service"
RESTART_UNIT = "datacenter-bot-restart-api.service"
# Root-owned instance adapters can set these before calling main(). They are
# never accepted from socket requests or inherited environment variables.
UPDATE_COMMAND = ["/usr/local/bin/datacenter-bot", "update"]
API_UNIT = "digital-ocean-bot-api"
DEPLOY_LOCK = "/opt/.digital-ocean-bot-deploy.lock"
STAGES = ["Queued", "Checking prerequisites", "Fetching release", "Installing dependencies",
          "Building release", "Backing up and preserving data", "Verifying services and panel", "Update complete"]


def now():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def prepare():
    STATE.mkdir(mode=0o700, parents=True, exist_ok=True)
    RUNTIME.mkdir(mode=0o755, parents=True, exist_ok=True)


@contextlib.contextmanager
def lock(name, nonblocking=False):
    fd = os.open(str(RUNTIME / name), os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    try:
        fcntl.flock(fd, fcntl.LOCK_EX | (fcntl.LOCK_NB if nonblocking else 0))
        yield
    finally:
        os.close(fd)


def save(state):
    fd, path = tempfile.mkstemp(prefix=".state-", dir=STATE)
    try:
        with os.fdopen(fd, "w") as out:
            json.dump(state, out)
            out.flush()
            os.fsync(out.fileno())
        os.replace(path, STATE / "state.json")
        directory = os.open(str(STATE), os.O_DIRECTORY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    finally:
        if os.path.exists(path):
            os.unlink(path)


def read_state():
    path = STATE / "state.json"
    if not path.exists():
        return {"status": "idle", "stage": 0}
    return json.loads(path.read_text())


def unit_active(unit):
    result = subprocess.run(["systemctl", "show", unit, "-p", "ActiveState", "--value"],
                            capture_output=True, text=True, timeout=5)
    if result.returncode:
        raise RuntimeError("service_status_unavailable")
    return result.stdout.strip() in ("active", "activating", "deactivating", "reloading")


def revision():
    value = json.loads(MANIFEST.read_text()).get("commit", "")
    if not re.fullmatch(r"[0-9a-f]{40}", value):
        raise RuntimeError("installed_revision_unavailable")
    return value


def status():
    state = read_state()
    active = unit_active(UPDATE_UNIT)
    if not active:
        state = read_state()
    if state.get("status") in ("queued", "running") and not active:
        # A queued job may not yet have been dispatched by systemd.
        age = time.time() - state.get("queued_epoch", 0)
        if age > 15:
            state.update(status="interrupted", ended_at=now(), error="update_interrupted",
                         recovery_required=PENDING.exists())
            save(state)
    # Return only reviewed fields. Logs and private paths never cross the socket.
    allowed = ("job_id", "status", "stage", "started_at", "ended_at",
               "from_revision", "installed_revision", "error", "recovery_required")
    result = {key: state[key] for key in allowed if key in state}
    # A CLI deployment can supersede the last panel job. Report the installed
    # manifest on every read, while retaining the job history separately.
    result["installed_revision"] = revision()
    result.update(available=True, busy=active or state.get("status") in ("queued", "running"),
                  message=STAGES[min(7, max(0, int(state.get("stage", 0))))])
    return result


def start():
    state = status()
    if state["busy"]:
        return state
    if PENDING.exists():
        raise ValueError("recovery_required")
    # Refuse a competing CLI install before accepting an update job.
    fd = os.open(INSTALLER_LOCK, os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    try:
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise ValueError("deployment_busy") from None
        state = {"job_id": uuid.uuid4().hex, "status": "queued", "stage": 0,
                 "queued_epoch": time.time(), "started_at": now(), "from_revision": revision()}
        save(state)
    finally:
        os.close(fd)
    try:
        subprocess.run(["systemctl", "start", "--no-block", UPDATE_UNIT],
                       check=True, capture_output=True, timeout=5)
    except (subprocess.SubprocessError, OSError):
        pass  # Durable queued state already exists; status reconciles uncertainty.
    return status()


def dispatch(request):
    if not isinstance(request, dict) or set(request) != {"action"}:
        raise ValueError("invalid_request")
    action = request["action"]
    if action not in ("status", "start", "restart_api"):
        raise ValueError("invalid_action")
    with lock("control.lock"):
        if action == "status":
            return status()
        if action == "start":
            return start()
        if status()["busy"] or PENDING.exists():
            raise ValueError("deployment_busy")
        subprocess.run(["systemctl", "restart", "--no-block", RESTART_UNIT],
                       check=True, capture_output=True, timeout=5)
        return {"available": True, "status": "restart_queued", "stage": 0, "busy": False}


def strict_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("invalid_request")
        result[key] = value
    return result


def control():
    conn = socket.socket(fileno=os.dup(0))
    conn.settimeout(5)
    try:
        _, uid, _ = struct.unpack("3i", conn.getsockopt(socket.SOL_SOCKET, socket.SO_PEERCRED, 12))
        if uid not in (0, pwd.getpwnam("digitaloceanbot").pw_uid):
            raise ValueError("forbidden")
        data = b""
        while b"\n" not in data and len(data) <= 4096:
            chunk = conn.recv(min(4097 - len(data), 1024))
            if not chunk:
                break
            data += chunk
        if len(data) > 4096 or not data.endswith(b"\n") or data.count(b"\n") != 1:
            raise ValueError("invalid_request")
        request = json.loads(data.decode("utf-8"), object_pairs_hook=strict_object)
        result = {"ok": True, "state": dispatch(request)}
    except ValueError as error:
        code = str(error)
        if code not in ("invalid_request", "invalid_action", "forbidden", "deployment_busy", "recovery_required"):
            code = "invalid_request"
        result = {"ok": False, "error": code}
    except Exception:
        result = {"ok": False, "error": "update_control_unavailable"}
    try:
        conn.sendall((json.dumps(result) + "\n").encode())
    finally:
        conn.close()


def run_update():
    with lock("job.lock", nonblocking=True):
        state = read_state()
        if state.get("status") != "queued" or not re.fullmatch(r"[0-9a-f]{32}", state.get("job_id", "")):
            raise RuntimeError("no_queued_job")
        state.update(status="running", stage=1)
        state.pop("error", None)
        save(state)
        try:
            environment = {"PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
                           "HOME": "/root", "LANG": "C.UTF-8", "DEBIAN_FRONTEND": "noninteractive"}
            with (STATE / (state["job_id"] + ".log")).open("w") as log:
                os.chmod(log.name, 0o600)
                process = subprocess.Popen(UPDATE_COMMAND,
                                           stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                                           stderr=subprocess.STDOUT, text=True, env=environment)
                for line in process.stdout:
                    log.write(line)
                    log.flush()
                    match = re.match(r"^\[([1-7])/7\]", line)
                    if match:
                        state["stage"] = int(match.group(1))
                        save(state)
                code = process.wait()
            if code != 0:
                raise RuntimeError("installer_failed")
            state.update(status="succeeded", stage=7, installed_revision=revision(), ended_at=now())
        except Exception:
            state.update(status="failed", error="update_failed", ended_at=now(), recovery_required=PENDING.exists())
            save(state)
            raise
        save(state)


def restart_api():
    # Wait for the settings response, then serialize with all deployment writers.
    time.sleep(2)
    fd = os.open(DEPLOY_LOCK, os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    try:
        fcntl.flock(fd, fcntl.LOCK_EX)
        if PENDING.exists():
            raise RuntimeError("recovery_required")
        subprocess.run(["systemctl", "restart", API_UNIT], check=True, timeout=60)
    finally:
        os.close(fd)


def main():
    os.umask(0o077)
    prepare()
    mode = sys.argv[1] if len(sys.argv) == 2 else ""
    if mode == "control":
        control()
    elif mode == "run":
        run_update()
    elif mode == "restart-api":
        restart_api()
    else:
        raise SystemExit(64)


if __name__ == "__main__":
    main()
