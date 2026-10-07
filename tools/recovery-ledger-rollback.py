#!/usr/bin/env python3
"""Restore the saved split-worker binaries only after durable work is settled."""
import argparse, json, os, pathlib, shutil, subprocess

UNITS = ["digital-ocean-bot-worker", "digital-ocean-bot-worker-panels"]

def run(args):
    return subprocess.check_output(args, text=True, stderr=subprocess.STDOUT).strip()

def ensure_no_pending():
    p = subprocess.run(["psql", os.environ["DATABASE_URL"], "-XAt", "-v", "ON_ERROR_STOP=1",
        "-c", "SELECT count(*) FROM worker_recovery_checkpoints"], capture_output=True, text=True,
        timeout=10, env={**os.environ, "PGOPTIONS": "-c default_transaction_read_only=on -c statement_timeout=5000"})
    if p.returncode or p.stdout.strip() != "0":
        raise RuntimeError("ROLLBACK_BLOCKED: unresolved or unreadable recovery checkpoints; reconcile with checkpoint-aware code first")

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--check-only", action="store_true")
    parser.add_argument("--backup-dir", type=pathlib.Path)
    args = parser.parse_args()
    if args.check_only:
        ensure_no_pending()
        print("ROLLBACK_CHECKPOINT_GATE_PASS")
        return
    if args.backup_dir is None:
        parser.error("--backup-dir required")
    b = args.backup_dir
    app = pathlib.Path("/opt/digital-ocean-bot")
    for name in ["api.before", "worker.before", "manifest.before"]:
        if not (b/name).is_file():
            raise RuntimeError("rollback backup missing: "+name)
    # Both producers must be stopped before inspecting the durable fence.
    for unit in UNITS:
        run(["systemctl", "stop", unit])
        if run(["systemctl", "show", unit, "-p", "MainPID", "--value"]) != "0":
            raise RuntimeError("ROLLBACK_BLOCKED: worker has not stopped")
    ensure_no_pending()
    for role in ["api", "worker"]:
        target = app/"bin"/("digital-ocean-bot-"+role)
        temp = target.with_name(target.name+".recovery-rollback")
        shutil.copy2(b/(role+".before"), temp)
        info = target.stat()
        os.chown(temp, info.st_uid, info.st_gid)
        temp.chmod(info.st_mode & 0o777)
        os.replace(temp, target)
    shutil.copy2(b/"manifest.before", app/"build-manifest.json")
    # Keep additive schema162 and the existing split topology. Never drop data.
    run(["systemctl", "restart", "digital-ocean-bot-api"])
    run(["systemctl", "start", *UNITS])
    print("ROLLED_BACK_SPLIT_ZERO_PENDING_CHECKPOINTS")

if __name__ == "__main__":
    main()
