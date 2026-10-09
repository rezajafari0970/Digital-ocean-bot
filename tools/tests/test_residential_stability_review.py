"""Adversarial regressions required by independent review F1-F11."""
import copy
import datetime as dt
import importlib.util
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch
import test_residential_stability_pilot as base

m = base.m
spec = importlib.util.spec_from_file_location("traffic", base.ROOT / "tools/residential-pilot-traffic.py")
traffic = importlib.util.module_from_spec(spec)
spec.loader.exec_module(traffic)

class WorkflowReviewTests(unittest.TestCase):
    setUp = base.WorkflowTests.setUp
    tearDown = base.WorkflowTests.tearDown
    engine = base.WorkflowTests.engine
    run_engine = base.WorkflowTests.run_engine

    def test_runtime_change_before_uncommitted_resume_denies_start(self):
        with patch("builtins.print"): self.engine().prepare()
        self.a.runtime_changed = True
        result = self.run_engine()
        self.assertEqual(result["phase"], "RECOVERY_PENDING")
        self.assertEqual(self.a.calls, [])
        self.assertEqual(self.a.collect_count, 2)

    def test_zero_success_baseline_stops_before_qualification(self):
        original = self.a.traffic
        def empty(label, until=None):
            value = original(label, until)
            for row in value["cases"]:
                if row["case"] in m.STATIC:
                    row.update(passed=False, http=0, curl_code=28)
            return value
        self.a.traffic = empty
        result = self.run_engine()
        self.assertEqual(result["phase"], "NO_ACTION")
        self.assertEqual(self.a.calls, [])
        self.assertEqual(self.a.collect_count, 0)

    def test_live_assignment_disappearance_is_not_deletion(self):
        current = self.a.snapshot()
        other = next(x["panel_id"] for x in current["assignments"] if x["panel_id"] != self.p["panel"])
        current["assignments"] = [x for x in current["assignments"] if x["panel_id"] != other]
        with self.assertRaises(m.Gate): m.unchanged(self.p, current)
        current["inventory"][other] = "DELETED"
        m.unchanged(self.p, current)
        current["inventory"].pop(other)
        with self.assertRaises(m.Gate): m.unchanged(self.p, current)

    def test_total_candidate_budget_includes_apply_native_and_reconciliation(self):
        for delay, native_delay in [(80, 20), (91, 0)]:
            with self.subTest(delay=delay, native=native_delay):
                for file in self.d.glob("*.json"): file.unlink()
                self.c = base.Clock()
                self.a = base.Fake(self.p, self.c)
                original = self.a.snapshot
                delayed = [False]
                def snapshot():
                    if self.a.calls and not delayed[0]:
                        delayed[0] = True
                        self.c.sleep(delay)
                    return original()
                self.a.snapshot = snapshot
                native = self.a.native
                def native_check(until=None):
                    result = native(until)
                    if self.a.native_calls == 2: self.c.sleep(native_delay)
                    return result
                self.a.native = native_check
                result = self.run_engine()
                self.assertEqual(result["trial_outcome"], "rejected")
                self.assertNotEqual(result["restoration_outcome"], "not_required")

    def test_owned_tuning_base_or_evidence_mismatch(self):
        self.a.crash = True
        with self.assertRaises(base.Crash), patch("builtins.print"): self.engine().run()
        snap = self.a.snapshot()
        state = json.loads((self.d / "state.json").read_text())
        for key, value in [("base_version", 777), ("base_revision", 777), ("admission_evidence_id", str(base.uuid.uuid4()))]:
            altered = copy.deepcopy(snap)
            altered["profile"]["tuning"][key] = value
            with self.subTest(key=key), self.assertRaises(m.Gate): m.active(self.p, altered, state["request"])

    def test_restored_generation_with_different_parent_plan_is_pending(self):
        self.a.crash = True
        with self.assertRaises(base.Crash), patch("builtins.print"): self.engine().run()
        self.a.current["profile"]["tuning"]["phase"] = "RESTORING"
        original = self.a.snapshot
        def changed_plan():
            result = original()
            result["panel"]["plan_hash"] = "different-native-parent"
            return result
        self.a.snapshot = changed_plan
        result = self.run_engine()
        self.assertEqual(result["phase"], "RECOVERY_PENDING")
        self.assertNotEqual(result["restoration_outcome"], "verified")

class OracleReviewTests(unittest.TestCase):
    def test_all_records_classes_flags_hashes_and_numeric_types(self):
        for kind in ["failed-malformed", "wrong-class", "false-flag", "missing-hash", "equal-hash", "false-distinct", "bool-time"]:
            value = base.fixture("traffic-candidate")
            static = next(x for x in value["cases"] if x["case"] == "ads-0")
            if kind == "failed-malformed":
                failed = next(x for x in value["cases"] if x["case"] in m.STATIC and not x["passed"])
                failed["curl_code"] = "timeout"
            elif kind == "wrong-class": static["class"] = "DIRECT"
            elif kind == "false-flag": static["passed"] = False
            elif kind == "missing-hash": next(x for x in value["cases"] if x["case"] == "egress-res")["egress_sha256"] = None
            elif kind == "equal-hash":
                values = [x for x in value["cases"] if x["case"].startswith("egress-")]
                values[0]["egress_sha256"] = values[1]["egress_sha256"]
            elif kind == "false-distinct": value["egress_distinct"] = False
            else: static["seconds"] = True
            with self.subTest(kind=kind), self.assertRaises(m.Gate): m.score(value)

    def test_readonly_cancel_reconciliation_normalizes_absent_evidence(self):
        with tempfile.TemporaryDirectory() as directory:
            adapter = m.Adapter({}, directory)
            adapter.tools["tune"] = "/unused"
            adapter.command = lambda *a, **kw: (0, json.dumps({"found": True, "exact_request_verified": True, "receipt": {"version": 7}, "evidence_id": ""}))
            value = adapter.lookup({"request_id": "unused"})
            self.assertIsNone(value["evidence_id"])
            self.assertTrue(value["exact_request_verified"])

    def test_exact_native_result(self):
        with tempfile.TemporaryDirectory() as directory:
            adapter = m.Adapter({"panel": "selected"}, directory)
            adapter.tools["native"] = "/unused"
            valid = "ADS_UDP_DNS_PASS panel=selected route_proofs=106\nADS_UDP_DNS_FLEET_PROOF panels=1 passed=1 pending=0 at=2026-10-09T12:00:00Z\n"
            adapter.command = lambda *a, **kw: (0, valid)
            adapter.native()
            for value in [valid.replace("=106", "=1060"), valid + valid,
                          valid + "ADS_UDP_DNS_PENDING panel=selected reason=failed\n",
                          valid.replace("panel=selected", "panel=other")]:
                adapter.command = lambda *a, _value=value, **kw: (0, _value)
                with self.assertRaises(m.Gate): adapter.native()

    def test_ipv4_parser_rejects_arbitrary_and_ipv6(self):
        self.assertEqual(m.support.ipv4("1.2.3.4"), "1.2.3.4")
        for value in ["garbage", "::1", "2001:db8::1", "1.2.3.400", "1.2.3.4 extra"]:
            with self.assertRaises(ValueError): m.support.ipv4(value)

    def test_partial_journal_survives_final_write_failure(self):
        with tempfile.TemporaryDirectory() as directory:
            evidence = traffic.Evidence(directory, "panel", "partial")
            first = traffic.cases()[0]
            at = evidence.begin(first)
            result = {"case": first[0], "class": first[1], "at": at, "passed": False,
                      "http": 0, "curl_code": 28, "seconds": 6, "egress_sha256": None}
            evidence.finish(result)
            evidence.begin(traffic.cases()[1])
            with patch.object(traffic.support, "atomic", side_effect=OSError("disk full")):
                with self.assertRaises(OSError): evidence.finalize(False)
            events = [json.loads(line) for line in evidence.journal.read_text().splitlines()]
            self.assertEqual([x["event"] for x in events], ["started", "finished", "started"])
            partial = evidence.finalize(False)
            self.assertFalse(partial["complete"])
            with self.assertRaises(m.Gate): m.score(partial)
            with self.assertRaises(ValueError): traffic.Evidence(directory, "panel", "partial")

    def test_curl_timeout_is_durable_failed_attempt(self):
        import threading
        with tempfile.TemporaryDirectory() as directory:
            evidence = traffic.Evidence(directory, "panel", "timeout")
            with patch.object(traffic.subprocess, "run", side_effect=subprocess.TimeoutExpired("curl", 8)):
                row = traffic.attempt(traffic.cases()[0], 1, directory, evidence, threading.Event())
            self.assertEqual(row["outcome"], "interrupted")
            events = [json.loads(line) for line in evidence.journal.read_text().splitlines()]
            self.assertEqual(len(events), 2)
            self.assertFalse(events[-1]["passed"])

class ProcessReviewTests(unittest.TestCase):
    def live(self, pid):
        try: return Path("/proc", str(pid), "stat").read_text().rsplit(")", 1)[1].split()[0] not in ("Z", "X")
        except FileNotFoundError: return False

    def test_nonreading_stdin_is_bounded(self):
        with tempfile.TemporaryDirectory() as directory:
            started = time.monotonic()
            rc = m.support.bounded_command([sys.executable, "-c", "import time;time.sleep(30)"],
                    Path(directory) / "out", .2, os.environ.copy(), lambda: None, "x" * 262144)
            self.assertEqual(rc, 124)
            self.assertLess(time.monotonic() - started, 5)

    def test_exited_leader_and_sigterm_ignoring_descendant_are_cleaned(self):
        for leader_exits in [True, False]:
            with self.subTest(leader_exits=leader_exits), tempfile.TemporaryDirectory() as directory:
                pidfile = Path(directory) / "descendant"
                code = "import os,signal,time,sys\np=os.fork()\nif p==0:\n signal.signal(signal.SIGTERM,signal.SIG_IGN)\n open(sys.argv[1],'w').write(str(os.getpid()))\n time.sleep(30)\nelse:\n time.sleep(.1)\n" + (" os._exit(0)\n" if leader_exits else " time.sleep(30)\n")
                rc = m.support.bounded_command([sys.executable, "-c", code, str(pidfile)],
                        Path(directory) / "out", .3, os.environ.copy(), lambda: None)
                self.assertEqual(rc, 0 if leader_exits else 124)
                self.assertFalse(self.live(int(pidfile.read_text())))

    def test_supervisor_death_cleans_group_and_releases_lock(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            childfile, lockfile = root / "child", root / "lock"
            command = "import os,time,sys,signal;signal.signal(signal.SIGTERM,signal.SIG_IGN);open(sys.argv[1],'w').write(str(os.getpid()));time.sleep(30)"
            launcher = "from importlib.machinery import SourceFileLoader\nfrom pathlib import Path\nimport os,sys,fcntl\ns=SourceFileLoader('s',sys.argv[1]).load_module()\nf=os.open(sys.argv[2],os.O_CREAT|os.O_RDWR,0o600);fcntl.flock(f,fcntl.LOCK_EX)\ns.bounded_command([sys.executable,'-c',sys.argv[4],sys.argv[5]],Path(sys.argv[3]),30,os.environ.copy(),lambda:None)\n"
            parent = subprocess.Popen([sys.executable, "-c", launcher, str(base.ROOT / "tools/residential-pilot-support.py"), str(lockfile), str(root / "out"), command, str(childfile)])
            try:
                end = time.monotonic() + 3
                while not childfile.exists() and time.monotonic() < end: time.sleep(.02)
                self.assertTrue(childfile.exists())
                pid = int(childfile.read_text())
                parent.kill()
                parent.wait()
                end = time.monotonic() + 5
                while self.live(pid) and time.monotonic() < end: time.sleep(.05)
                self.assertFalse(self.live(pid))
                released=False
                while time.monotonic()<end:
                    try:
                        with m.lock(lockfile):
                            self.assertFalse(self.live(pid))
                            released=True
                        break
                    except BlockingIOError: time.sleep(.025)
                self.assertTrue(released,"cleanup must release the lock within its bounded budget")
            finally:
                if parent.poll() is None: parent.kill(); parent.wait()

    def test_zombie_is_not_reported_active(self):
        pid = os.fork()
        if pid == 0: os._exit(0)
        try:
            end = time.monotonic() + 2
            while self.live(pid) and time.monotonic() < end: time.sleep(.01)
            fields = Path("/proc", str(pid), "stat").read_text().rsplit(")", 1)[1].split()
            with tempfile.TemporaryDirectory() as directory:
                m.atomic(Path(directory) / "status.json", {
                    "pid": pid, "pid_start_ticks": fields[19],
                    "boot_id": Path("/proc/sys/kernel/random/boot_id").read_text().strip(),
                    "valid_until": (m.now() + dt.timedelta(seconds=90)).isoformat()})
                self.assertFalse(m.status(directory)["activity_confirmed"])
        finally: os.waitpid(pid, 0)

    def test_protected_libpq_files_keep_dsn_and_password_out_of_argv(self):
        with tempfile.TemporaryDirectory() as directory:
            dsn = "postgresql://example:secret%3Awith%5Cescape@127.0.0.1:5432/example?sslmode=disable"
            with m.support.database_environment(directory, {"DATABASE_URL": dsn, "PATH": os.environ["PATH"]}) as env:
                self.assertNotIn("DATABASE_URL", env)
                self.assertNotIn("PGPASSWORD", env)
                service = Path(env["PGSERVICEFILE"])
                self.assertEqual(service.stat().st_mode & 0o777, 0o600)
                self.assertNotIn("secret", service.read_text())
                self.assertIn("passfile=", service.read_text())
                child = subprocess.Popen([sys.executable, "-c", "import time;time.sleep(2)"], env=env)
                try:
                    argv = Path("/proc", str(child.pid), "cmdline").read_bytes()
                    self.assertNotIn(dsn.encode(), argv)
                    self.assertNotIn(b"secret", argv)
                finally: child.terminate(); child.wait()
            self.assertEqual(list(Path(directory).iterdir()), [])

    def test_actual_collector_interruption_retains_attempts(self):
        # Actual collector in a separate process; external DB/network are isolated doubles.
        # TERM must materialize partial evidence before blocked futures finish.
        # KILL cannot finalize, but completed and started records must survive.
        launcher = r"""
import contextlib, importlib.util, json, pathlib, subprocess, sys, threading, time
from unittest.mock import patch
spec=importlib.util.spec_from_file_location("traffic_child",sys.argv[1])
t=importlib.util.module_from_spec(spec);spec.loader.exec_module(t)
@contextlib.contextmanager
def db(*a): yield {}
class Core:
 def poll(self): return None
 def terminate(self): pass
 def wait(self,**kw): return 0
 def kill(self): pass
@contextlib.contextmanager
def connection(*a,**kw): yield object()
lock=threading.Lock()
count=0
def command(args,**kw):
 global count
 if args[0]=="psql":
  rows=[{"route_class":c,"uri":"vless://00000000-0000-0000-0000-000000000001@127.0.0.1:443?security=reality&type=tcp"} for c in ["DIRECT","RESIDENTIAL"]]
  return subprocess.CompletedProcess(args,0,json.dumps(rows),"")
 with lock:
  count+=1; n=count
 if n>3: time.sleep(20)
 return subprocess.CompletedProcess(args,0,json.dumps({"http_code":200,"time_total":.1}),"")
with patch.object(t.support,"database_environment",db),patch.object(t.subprocess,"run",command),patch.object(t.subprocess,"Popen",lambda *a,**k:Core()),patch.object(t.socket,"create_connection",connection):
 t.run("00000000-0000-0000-0000-000000000001","interrupted",sys.argv[2],"/isolated/core")
"""
        for sig in (signal.SIGTERM, signal.SIGKILL):
            with self.subTest(signal=sig), tempfile.TemporaryDirectory() as directory:
                root=Path(directory); (root/"tmp").mkdir()
                child=subprocess.Popen([sys.executable,"-c",launcher,str(base.ROOT/"tools/residential-pilot-traffic.py"),directory],
                                       stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
                journal=root/"interrupted.events.jsonl"
                try:
                    end=time.monotonic()+5
                    events=[]
                    while time.monotonic()<end:
                        if journal.exists():
                            events=[json.loads(line) for line in journal.read_text().splitlines()]
                            if sum(x["event"]=="finished" for x in events)>=3 and sum(x["event"]=="started" for x in events)>=4: break
                        time.sleep(.02)
                    self.assertGreaterEqual(sum(x["event"]=="finished" for x in events),3)
                    child.send_signal(sig)
                    if sig==signal.SIGTERM:
                        report=root/"interrupted.json"
                        end=time.monotonic()+3
                        while not report.exists() and time.monotonic()<end: time.sleep(.02)
                        self.assertTrue(report.exists(),"TERM must persist before waiting for futures")
                        value=json.loads(report.read_text())
                        self.assertFalse(value["complete"])
                        with self.assertRaises(m.Gate): m.score(value)
                    else:
                        child.wait(timeout=3)
                        self.assertFalse((root/"interrupted.json").exists())
                    retained=[json.loads(line) for line in journal.read_text().splitlines()]
                    self.assertGreaterEqual(sum(x["event"]=="finished" for x in retained),3)
                    self.assertGreaterEqual(sum(x["event"]=="started" for x in retained),4)
                    with self.assertRaises(ValueError): traffic.Evidence(root,"panel","interrupted")
                finally:
                    if child.poll() is None: child.kill()
                    child.wait(timeout=3)
