#!/usr/bin/env python3
"""Root-owned production instance installer; never changes the default instance."""
import contextlib, datetime, fcntl, hashlib, json, os, pathlib, re, runpy
import shutil, signal, subprocess, sys, tempfile, time, urllib.request

ROOT = pathlib.Path("/opt/dob-production")
ETC = pathlib.Path("/etc/dob-production")
STATE = pathlib.Path("/var/lib/dob-production-updater")
CACHE = pathlib.Path("/var/lib/dob-production-installer")
UNITS = ["dob-production-worker-panels", "dob-production-worker", "dob-production-api"]
TIMER = "dob-production-integrity.timer"
PENDING = ETC / "deployment.pending"
HEALTH = pathlib.Path("/var/lib/dob-production-health/integrity.json")
REPOSITORY = "https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config.git"
VERIFY = "/usr/local/libexec/dob-production/verify.py"

def run(args, **kwargs):
    return subprocess.run(args, check=True, **kwargs)

def atomic(path, value, mode=0o600):
    fd, name = tempfile.mkstemp(prefix=".write-", dir=path.parent)
    try:
        with os.fdopen(fd, "w") as f:
            os.fchmod(f.fileno(), mode)
            json.dump(value, f, indent=2)
            f.flush()
            os.fsync(f.fileno())
        os.replace(name, path)
        fd = os.open(path.parent, os.O_DIRECTORY)
        try: os.fsync(fd)
        finally: os.close(fd)
    finally:
        if os.path.exists(name): os.unlink(name)

@contextlib.contextmanager
def locks():
    handles = []
    try:
        for name in ["/run/lock/dob-production-installer.lock", "/opt/.dob-production-deploy.lock"]:
            fd = os.open(name, os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
            handles.append(fd)
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        yield
    finally:
        for fd in reversed(handles): os.close(fd)

def manifest(root=ROOT):
    value = json.loads((root / "build-manifest.json").read_text())
    if not re.fullmatch("[0-9a-f]{40}", value.get("commit", "")):
        raise RuntimeError("invalid_release_identity")
    return value

def verify_runtime(root):
    m = manifest(root)
    for name, expected in [("digital-ocean-bot-api", m["api_sha256"]),
                            ("digital-ocean-bot-worker", m["worker_sha256"])] + [
        ("server-guardian-linux-" + arch, sha) for arch, sha in m["guardian_sha256"].items()]:
        p = root / "bin" / name
        if p.is_symlink() or hashlib.sha256(p.read_bytes()).hexdigest() != expected:
            raise RuntimeError("runtime_hash_mismatch")
    for name in ["bootstrap.py", "database-integrity.py", "backup_store.py", "admission-compatibility.py"]:
        if not (root / "support" / name).is_file(): raise RuntimeError("support_missing")
    return m

def api(path):
    with urllib.request.urlopen("http://127.0.0.1:28080" + path, timeout=5) as f:
        return json.load(f)

def ready(expected, seconds=300):
    end = time.monotonic() + seconds
    while time.monotonic() < end:
        try:
            if api("/readyz")["status"] == "ready" and api("/version")["commit"] == expected:
                m = manifest()
                for unit in UNITS:
                    pid = int(subprocess.check_output(["systemctl", "show", unit, "-p", "MainPID", "--value"], text=True))
                    key = "api_sha256" if unit.endswith("-api") else "worker_sha256"
                    if not pid or hashlib.sha256(pathlib.Path("/proc", str(pid), "exe").read_bytes()).hexdigest() != m[key]:
                        raise RuntimeError("running_artifact_mismatch")
                return
        except Exception: pass
        time.sleep(2)
    raise RuntimeError("instance_not_ready")

def stage():
    print("[2/7] Fetching the approved stable release", flush=True)
    source = pathlib.Path(tempfile.mkdtemp(prefix="source-", dir=CACHE))
    run(["git", "-c", "core.hooksPath=/dev/null", "clone", "--quiet", "--depth", "1", "--branch", "stable", REPOSITORY, str(source)], timeout=300)
    commit = subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip()
    if not re.fullmatch("[0-9a-f]{40}", commit): raise RuntimeError("invalid_source_commit")
    if commit == manifest()["commit"]:
        shutil.rmtree(source)
        return None
    print("[3/7] Checking pinned toolchain and installed dependencies", flush=True)
    result = CACHE / ("staged-" + commit + ".json")
    if result.exists(): raise RuntimeError("unreviewed_staged_candidate")
    environment = dict(os.environ, SRC=str(source), APP=str(ROOT), BUILD_COMMIT=commit,
                       BUILD_TIME=datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
                       DOB_STAGE_RESULT=str(result))
    print("[4/7] Building independent production artifacts", flush=True)
    script = """
set -Eeuo pipefail
source "$SRC/deploy/toolchain.sh"
dob_install_toolchain
source "$SRC/deploy/service-topology.sh"
LDFLAGS="-s -w -X github.com/rezajafari0970/Digital-ocean-bot/internal/buildinfo.Commit=$BUILD_COMMIT -X github.com/rezajafari0970/Digital-ocean-bot/internal/buildinfo.BuildTime=$BUILD_TIME"
dob_stage_runtime
install -d -m 0750 "$DOB_STAGE/support"
for asset in bootstrap.py database-integrity.py backup_store.py admission-compatibility.py; do
 install -m 0640 "$SRC/deploy/$asset" "$DOB_STAGE/support/$asset"
done
dob_prepare_stage_permissions
python3 - "$DOB_STAGE_RESULT" "$DOB_STAGE" "$SRC" <<'RESULT'
import json,pathlib,sys
pathlib.Path(sys.argv[1]).write_text(json.dumps(dict(stage=sys.argv[2],source=sys.argv[3])))
RESULT
trap - EXIT
"""
    run(["bash", "-c", script], env=environment, timeout=1200)
    candidate = pathlib.Path(json.loads(result.read_text())["stage"])
    if verify_runtime(candidate)["commit"] != commit: raise RuntimeError("candidate_identity_changed")
    return candidate

def migrate():
    h = runpy.run_path(str(ROOT / "support/bootstrap.py"))
    env = dict(os.environ, **h["read_environment"](ETC / "env"))
    env.pop("DOB_FRESH_INSTALL", None)
    env.pop("PGOPTIONS", None)
    with (STATE / "migration.log").open("w") as log:
        p = subprocess.Popen(["/usr/sbin/runuser", "-u", "digitaloceanbot", "--",
                              str(ROOT / "bin/install-bootstrap")], cwd=ROOT, env=env,
                             stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
        try:
            code = p.wait(timeout=930)
        except subprocess.TimeoutExpired:
            os.killpg(p.pid, signal.SIGTERM)
            try: p.wait(timeout=30)
            except subprocess.TimeoutExpired:
                os.killpg(p.pid, signal.SIGKILL)
                p.wait()
            raise RuntimeError("migration_timeout")
        if code: raise RuntimeError("migration_failed")

def deploy(candidate):
    expected = verify_runtime(candidate)["commit"]
    prior = manifest()["commit"]
    identity = json.loads((ETC / "instance.json").read_text())
    checks = {name: hashlib.sha256((ETC / name).read_bytes()).hexdigest() for name in ["env", "master.key"]}
    print("[5/7] Checking integrity and preserving existing data", flush=True)
    run(["python3", VERIFY], timeout=45)
    # This creates and seals a fresh logical backup, then archives it encrypted.
    run(["systemctl", "start", "dob-production-integrity.service"], timeout=1800)
    health = json.loads(HEALTH.read_text())
    if health.get("recovery_required") or health.get("status") != "ok" or not health.get("backup", {}).get("ok"):
        raise RuntimeError("integrity_recovery_required")
    h = runpy.run_path(str(ROOT / "support/bootstrap.py"))
    env = h["database_environment"](h["read_environment"](ETC / "env"))
    size = int(subprocess.check_output(["psql", "-XqAt", "-v", "ON_ERROR_STOP=1", "-c",
               "SELECT pg_database_size(current_database())"], env=env, text=True))
    if shutil.disk_usage(ROOT).free < size * 3 + 512 * 1024**2:
        raise RuntimeError("insufficient_retention_workspace")
    run(["python3", str(candidate / "support/admission-compatibility.py"),
         str(candidate / "support/bootstrap.py"), str(ETC / "env"), str(candidate / "build-manifest.json")], timeout=45)
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%S")
    previous = pathlib.Path("/opt/dob-production.rollback-" + stamp)
    failed = pathlib.Path("/opt/dob-production.failed-" + stamp)
    record = dict(from_revision=prior, to_revision=expected, previous=str(previous),
                  candidate=str(candidate), identity=identity, phase="prepared")
    atomic(PENDING, record)
    moved = False
    try:
        run(["systemctl", "stop", TIMER, "dob-production-integrity.service"], timeout=120)
        run(["systemctl", "stop"] + UNITS, timeout=120)
        for unit in UNITS:
            if subprocess.check_output(["systemctl", "show", unit, "-p", "MainPID", "--value"], text=True).strip() != "0":
                raise RuntimeError("runtime_still_active")
        ROOT.rename(previous)
        moved = True
        candidate.rename(ROOT)
        record["phase"] = "migrating"
        atomic(PENDING, record)
        migrate()
        for name, digest in checks.items():
            if hashlib.sha256((ETC / name).read_bytes()).hexdigest() != digest:
                raise RuntimeError("configuration_identity_changed")
        atomic(ETC / "instance.json", dict(identity, source_commit=expected), 0o644)
        record["phase"] = "verifying"
        atomic(PENDING, record)
        print("[6/7] Verifying isolated services and exact running artifacts", flush=True)
        run(["systemctl", "start", "--no-block"] + UNITS, timeout=15)
        ready(expected)
        atomic(STATE / "deployment-result.json", dict(ok=True, commit=expected, previous=str(previous),
                                                    at=datetime.datetime.now(datetime.timezone.utc).isoformat()))
        PENDING.unlink()
        print("[7/7] Update complete", flush=True)
    except BaseException as error:
        run(["systemctl", "stop"] + UNITS, timeout=120)
        if moved:
            if ROOT.exists(): ROOT.rename(failed)
            previous.rename(ROOT)
        atomic(ETC / "instance.json", identity, 0o644)
        record["phase"] = "rolling_back"
        atomic(PENDING, record)
        run(["systemctl", "start", "--no-block"] + UNITS, timeout=15)
        ready(prior)
        atomic(STATE / "deployment-result.json", dict(ok=False, rollback_ready=True,
                                                    commit=prior, error=type(error).__name__))
        PENDING.unlink()
        raise
    finally:
        run(["systemctl", "start", TIMER], timeout=15)


def recover():
    if not PENDING.exists(): raise RuntimeError("no_pending_deployment")
    record=json.loads(PENDING.read_text())
    prior=record["from_revision"]
    previous=pathlib.Path(record["previous"])
    if not re.fullmatch(r"/opt/dob-production.rollback-[0-9]{8}T[0-9]{6}",str(previous)):
        raise RuntimeError("invalid_recovery_path")
    run(["systemctl","stop",TIMER,"dob-production-integrity.service"]+UNITS,timeout=120)
    if previous.exists():
        if ROOT.exists():
            destination=pathlib.Path("/opt/dob-production.failed-recovery-"+datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%S"))
            ROOT.rename(destination)
        previous.rename(ROOT)
    if manifest()["commit"]!=prior: raise RuntimeError("recovery_runtime_unavailable")
    atomic(ETC/"instance.json",record["identity"],0o644)
    record["phase"]="rolling_back";atomic(PENDING,record)
    run(["systemctl","start","--no-block"]+UNITS,timeout=15)
    ready(prior)
    atomic(STATE/"deployment-result.json",dict(ok=False,rollback_ready=True,recovered=True,commit=prior))
    PENDING.unlink()
    run(["systemctl","start",TIMER],timeout=15)
    print("Previous production runtime restored and verified",flush=True)

def main():
    if os.geteuid() != 0: raise SystemExit("root_required")
    if sys.argv[1:] not in (["update"],["recover"]): raise SystemExit("Usage: dob-production {update|recover}")
    os.umask(0o077)
    os.environ["PATH"] = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
    for path in [STATE, CACHE]: path.mkdir(mode=0o700, parents=True, exist_ok=True)
    with locks():
        if sys.argv[1]=="recover":
            recover()
            return
        if PENDING.exists(): raise RuntimeError("recovery_required")
        print("[1/7] Checking production identity", flush=True)
        run(["python3", VERIFY], timeout=45)
        candidate = stage()
        if candidate is None:
            ready(manifest()["commit"], seconds=30)
            print("[7/7] Current release already installed and ready", flush=True)
        else: deploy(candidate)

if __name__ == "__main__":
    main()
