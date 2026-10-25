#!/usr/bin/env python3
"""Inspect or resume the verified production candidate after restoring disk headroom."""
import argparse
import json
import os
from pathlib import Path
import re
import runpy
import shutil
import subprocess

EXPECTED = "8d838efa34acc2bd6004045ccf335cc22cb5b2ef"
PRIOR = "5dc2c25678698dbbe9ac1cfee7d18701c19213e7"

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--deploy", action="store_true")
    args = parser.parse_args()
    if os.geteuid() != 0:
        raise SystemExit("root_required")
    os.umask(0o077)
    os.environ["PATH"] = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
    h = runpy.run_path("/usr/local/sbin/dob-production")
    with h["locks"]():
        if h["PENDING"].exists() or h["manifest"]()["commit"] != PRIOR:
            raise SystemExit("production_state_changed_review_required")
        record = h["CACHE"] / ("staged-" + EXPECTED + ".json")
        if record.is_symlink() or record.stat().st_uid != 0:
            raise SystemExit("unsafe_stage_record")
        candidate = Path(json.loads(record.read_text())["stage"])
        if (candidate.parent != Path("/opt") or candidate.is_symlink()
                or not re.fullmatch(r"\.dob-release\.[A-Za-z0-9]+", candidate.name)):
            raise SystemExit("unsafe_stage_path")
        if h["verify_runtime"](candidate)["commit"] != EXPECTED:
            raise SystemExit("candidate_identity_changed")
        h["run"](["python3", h["VERIFY"]], timeout=45)
        helper = runpy.run_path(str(h["ROOT"] / "support/bootstrap.py"))
        env = helper["database_environment"](helper["read_environment"](h["ETC"] / "env"))
        size = int(subprocess.check_output(
            ["psql", "-XqAt", "-v", "ON_ERROR_STOP=1", "-c",
             "SELECT pg_database_size(current_database())"], env=env, text=True, timeout=30))
        free = shutil.disk_usage(h["ROOT"]).free
        required = size * 3 + 512 * 1024**2
        print(json.dumps({"candidate": EXPECTED, "running": PRIOR, "free_bytes": free,
                          "reserve_bytes": required, "space_gate_now": free >= required,
                          "deploy_requested": args.deploy}), flush=True)
        if args.deploy:
            if free < required:
                raise SystemExit("insufficient_retention_workspace_services_untouched")
            # Fresh integrity/backup/archive and the original disk gate run again.
            # No retry, checksum bypass, backup deletion or policy override.
            h["deploy"](candidate)

if __name__ == "__main__":
    main()
