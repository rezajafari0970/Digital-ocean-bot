"""Private persistence, libpq connection files and bounded Linux process groups."""
import contextlib
import ctypes
import ctypes.util
import datetime as dt
import ipaddress
import json
import os
from pathlib import Path
import signal
import subprocess
import time
import uuid

def atomic(path, value):
    path = Path(path)
    tmp = path.with_name(path.name + "." + str(uuid.uuid4()) + ".tmp")
    try:
        fd = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        with os.fdopen(fd, "w") as stream:
            json.dump(value, stream, sort_keys=True, ensure_ascii=False, allow_nan=False)
            stream.write("\n")
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(tmp, path)
        fd = os.open(path.parent, os.O_DIRECTORY)
        try: os.fsync(fd)
        finally: os.close(fd)
    finally:
        if tmp.exists(): tmp.unlink()

def create_journal(path):
    path = Path(path)
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    try: os.fsync(fd)
    finally: os.close(fd)
    fd = os.open(path.parent, os.O_DIRECTORY)
    try: os.fsync(fd)
    finally: os.close(fd)

def append_event(path, event):
    fd = os.open(path, os.O_WRONLY | os.O_APPEND | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, "w") as stream:
        stream.write(json.dumps(event, sort_keys=True, allow_nan=False) + "\n")
        stream.flush()
        os.fsync(stream.fileno())

def ipv4(text):
    return str(ipaddress.IPv4Address(text))

class Option(ctypes.Structure):
    _fields_ = [(x, ctypes.c_char_p) for x in
                ("keyword", "envvar", "compiled", "val", "label", "dispchar")] + [("dispsize", ctypes.c_int)]

def connection_options(dsn):
    lib = ctypes.CDLL(ctypes.util.find_library("pq"))
    lib.PQconninfoParse.argtypes = [ctypes.c_char_p, ctypes.POINTER(ctypes.c_void_p)]
    lib.PQconninfoParse.restype = ctypes.POINTER(Option)
    lib.PQconninfoFree.argtypes = [ctypes.POINTER(Option)]
    lib.PQfreemem.argtypes = [ctypes.c_void_p]
    error = ctypes.c_void_p()
    options = lib.PQconninfoParse(dsn.encode(), ctypes.byref(error))
    if not options:
        if error.value: lib.PQfreemem(error)
        raise ValueError("protected database connection is invalid")
    result = {}
    try:
        index = 0
        while options[index].keyword:
            option = options[index]
            if option.val is not None:
                result[option.keyword.decode()] = option.val.decode()
            index += 1
    finally: lib.PQconninfoFree(options)
    return result

@contextlib.contextmanager
def database_environment(directory, environment):
    """Use libpq's own parser; neither DSN nor password is a child argv value."""
    root = Path(directory)
    values = connection_options(environment["DATABASE_URL"])
    password = values.pop("password", None)
    values["options"] = (values.get("options", "") + " -c default_transaction_read_only=on -c statement_timeout=10000").strip()
    token = str(uuid.uuid4())
    service = root / ("db-" + token + ".service")
    passfile = root / ("db-" + token + ".pgpass")
    def write(path, value):
        fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        with os.fdopen(fd, "w") as stream:
            stream.write(value)
            stream.flush()
            os.fsync(stream.fileno())
    try:
        if password is not None:
            if "\n" in password or "\r" in password:
                raise ValueError("unsupported protected database password format")
            escaped = password.replace("\\", "\\\\").replace(":", "\\:")
            write(passfile, "*:*:*:*:" + escaped + "\n")
            values["passfile"] = str(passfile)
        for key, value in values.items():
            if "\n" in value or "\r" in value or value != value.strip():
                raise ValueError("unsupported protected database option format")
        write(service, "[pilot]\n" + "".join(key + "=" + value + "\n" for key, value in values.items()))
        env = {k: v for k, v in environment.items() if k != "DATABASE_URL" and not k.startswith("PG")}
        env.update(PGSERVICEFILE=str(service), PGSERVICE="pilot",
                   PGOPTIONS="-c default_transaction_read_only=on -c statement_timeout=10000")
        yield env
    finally:
        for path in (service, passfile):
            if path.exists(): path.unlink()

def _signal_group(pgid, sig):
    try: os.killpg(pgid, sig)
    except ProcessLookupError: pass

def _reap():
    while True:
        try:
            pid, _ = os.waitpid(-1, os.WNOHANG)
            if pid == 0: return
        except ChildProcessError: return

def _group_alive(pgid):
    _reap()
    try: os.killpg(pgid, 0); return True
    except ProcessLookupError: return False

def _cleanup_group(pgid):
    _signal_group(pgid, signal.SIGTERM)
    end = time.monotonic() + 2
    while _group_alive(pgid) and time.monotonic() < end: time.sleep(.025)
    _signal_group(pgid, signal.SIGKILL)
    end = time.monotonic() + 1
    while _group_alive(pgid) and time.monotonic() < end: time.sleep(.025)
    _reap()

def _guard(parent, args, out_fd, input_fd, seconds, env, until, outpath):
    """Dedicated subreaper survives supervisor death long enough to clean its group."""
    libc = ctypes.CDLL(None)
    stopped = [False]
    def stop(*unused): stopped[0] = True
    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)
    libc.prctl(36, 1)  # PR_SET_CHILD_SUBREAPER
    libc.prctl(1, signal.SIGTERM)  # PR_SET_PDEATHSIG
    if os.getppid() != parent: os._exit(124)  # close registration race
    child = None
    code = 125
    started = time.monotonic()
    try:
        guard_pid = os.getpid()
        def child_setup():
            libc.prctl(1, signal.SIGKILL)
            if os.getppid() != guard_pid: os._exit(124)
        child = subprocess.Popen(args, stdin=input_fd, stdout=out_fd,
                                 stderr=subprocess.DEVNULL, env=env,
                                 start_new_session=True, preexec_fn=child_setup)
        while True:
            rc = child.poll()
            if stopped[0] or os.getppid() != parent or time.monotonic() - started >= seconds:
                code = 124
                break
            if until is not None and dt.datetime.now(dt.timezone.utc) >= until:
                code = 124
                break
            if outpath.stat().st_size > 2 * 1024 * 1024:
                code = 124
                break
            if rc is not None:
                code = rc if rc >= 0 else 128 - rc
                break
            time.sleep(.05)
    finally:
        if child is not None:
            _cleanup_group(child.pid)
            try: child.wait(timeout=.1)
            except (subprocess.TimeoutExpired, ChildProcessError): pass
    os._exit(min(max(code, 0), 255))

def bounded_command(args, outpath, seconds, environment, pulse, input_text=None, until=None):
    """Regular-file stdin cannot block on pipe delivery. Guard owns all tool children."""
    outpath = Path(outpath)
    input_path = outpath.with_suffix(".stdin")
    input_fd = os.open(os.devnull, os.O_RDONLY)
    out_fd = os.open(outpath, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    guard = None
    reaped = False
    try:
        if input_text is not None:
            data = input_text.encode()
            if len(data) > 2 * 1024 * 1024: raise ValueError("bounded input too large")
            fd = os.open(input_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
            with os.fdopen(fd, "wb") as stream: stream.write(data)
            os.close(input_fd)
            input_fd = os.open(input_path, os.O_RDONLY | os.O_NOFOLLOW)
        parent = os.getpid()
        guard = os.fork()
        if guard == 0:
            try: _guard(parent, args, out_fd, input_fd, seconds, environment, until, outpath)
            except BaseException: os._exit(125)
        while True:
            pulse()
            pid, status = os.waitpid(guard, os.WNOHANG)
            if pid:
                reaped = True
                return os.waitstatus_to_exitcode(status)
            time.sleep(.1)
    finally:
        os.close(input_fd)
        os.close(out_fd)
        if guard and not reaped:
            try: os.kill(guard, signal.SIGTERM)
            except ProcessLookupError: pass
            # Guard has a fixed three-second group cleanup budget.
            end = time.monotonic() + 4
            while time.monotonic() < end:
                try:
                    if os.waitpid(guard, os.WNOHANG)[0]: reaped = True; break
                except ChildProcessError: reaped = True; break
                time.sleep(.05)
            if not reaped:
                try: os.kill(guard, signal.SIGKILL)
                except ProcessLookupError: pass
                try: os.waitpid(guard, 0)
                except ChildProcessError: pass
        if input_path.exists(): input_path.unlink()
