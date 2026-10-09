"""Regressions for second review: authoritative lifecycle and durable bootstrap."""
import copy
import datetime as dt
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch
import test_residential_stability_pilot as base
import test_residential_stability_review as first
m=base.m
traffic=first.traffic

class LifecycleTests(unittest.TestCase):
    setUp=base.WorkflowTests.setUp
    tearDown=base.WorkflowTests.tearDown
    engine=base.WorkflowTests.engine
    run_engine=base.WorkflowTests.run_engine

    def test_server_rejection_cancel_or_deadline_drift_revokes_acceptance(self):
        for cause in ("verification", "early-cancel", "cancel-observed-after-deadline", "deadline-drift"):
            with self.subTest(cause=cause):
                for p in self.d.glob("*.json"): p.unlink()
                self.c=base.Clock();self.a=base.Fake(self.p,self.c)
                engine=self.engine()
                with patch("builtins.print"):
                    engine.prepare();engine.operation("request");engine.trial()
                self.assertEqual(engine.s["trial_outcome"],"accepted")
                original=engine.s["deadline"]
                tuning=self.a.current["profile"]["tuning"]
                tuning.update(phase="RESTORING",reason="operator cancelled")
                if cause=="verification":
                    tuning.update(rejected=True,reason="runtime verification failed")
                if cause=="cancel-observed-after-deadline": self.c.sleep(310)
                if cause=="deadline-drift":
                    tuning.update(reason="trial deadline expired",deadline=(m.instant(original)+dt.timedelta(minutes=1)).isoformat())
                    self.c.sleep(370)
                # New Engine instance models restart; no candidate remeasurement.
                result=self.run_engine()
                self.assertEqual(result["phase"],"COMPLETE")
                self.assertEqual(result["restoration_outcome"],"verified")
                self.assertEqual(result["trial_outcome"],"rejected")
                self.assertEqual(result["deadline"],original)
                self.assertEqual(self.a.collect_count,2)
                self.assertEqual(len(self.a.calls),1)

class OracleAndDurabilityTests(unittest.TestCase):
    def test_complete_must_be_explicit_boolean_true(self):
        for value in (None,False,1,"true"):
            report=base.fixture("traffic-candidate")
            if value is None: report.pop("complete")
            else: report["complete"]=value
            with self.subTest(value=value),self.assertRaises(m.Gate):m.score(report)

    def test_report_phase_must_match_requested_window(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory)
            report=base.fixture("traffic-candidate");report.update(panel="selected",phase="baseline")
            (root/"candidate.json").write_text(json.dumps(report))
            adapter=m.Adapter({"panel":"selected"},root)
            adapter.tools={"traffic":"/unused","xray":"/unused"}
            # Collector writes only when invoked; existing-report guard stays intact.
            (root/"candidate.json").unlink()
            def command(*a,**k):
                (root/"candidate.json").write_text(json.dumps(report))
                return 0,""
            adapter.command=command
            with self.assertRaises(m.Gate):adapter.traffic("candidate")

    def test_journal_file_and_directory_barriers_precede_dispatch(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);barriers=[];real=os.fsync
            def fsync(fd):
                barriers.append(os.readlink("/proc/self/fd/"+str(fd)))
                return real(fd)
            with patch.object(traffic.support.os,"fsync",side_effect=fsync):
                evidence=traffic.Evidence(root,"selected","barriers")
                expected=str(evidence.journal)
                index=barriers.index(expected)
                self.assertIn(str(root),barriers[index+1:])
                self.assertEqual(evidence.journal.read_text(),"")
                evidence.begin(traffic.cases()[0])
            self.assertEqual(json.loads(evidence.journal.read_text())["event"],"started")

class BootstrapTests(unittest.TestCase):
    @unittest.skipUnless(os.geteuid()==0,"protected staging requires operator account")
    def test_fresh_interpreter_uses_staged_support_before_any_execution(self):
        launcher=r"""
from importlib.machinery import SourceFileLoader
from pathlib import Path
import sys
m=SourceFileLoader("isolated_supervisor",sys.argv[1]).load_module()
real=m.lock
run_root=Path(sys.argv[2])
m.lock=lambda path,shared=False:real(run_root/Path(path).name,shared)
sys.argv=["pilot","--run","--directory",sys.argv[2]]
raise SystemExit(m.main())
"""
        for condition in ("missing-original","changed-original","corrupt-stage"):
            with self.subTest(condition=condition),tempfile.TemporaryDirectory() as directory:
                root=Path(directory);source=root/"source";source.mkdir()
                supervisor=source/"residential-stability-pilot.py"
                supervisor.write_bytes((base.ROOT/"tools/residential-stability-pilot.py").read_bytes())
                helper=source/"residential-pilot-support.py";helper.write_bytes(base.SUPPORT.read_bytes())
                artifact=source/"tool";artifact.write_text("raise RuntimeError('no tool may execute')\n")
                for p in (helper,artifact):p.chmod(0o500)
                fixture=base.WorkflowTests();fixture.setUp()
                try: policy=copy.deepcopy(fixture.p)
                finally:fixture.tearDown()
                policy["tools"]={name:{"path":str(helper if name=="support" else artifact),"sha256":m.hashlib.sha256((helper if name=="support" else artifact).read_bytes()).hexdigest()} for name in m.TOOLS}
                run=root/"run";run.mkdir(mode=0o700)
                adapter=m.Adapter(policy,run);adapter.stage()
                m.atomic(run/"policy.json",policy)
                m.atomic(run/"state.json",{"schema":1,"policy_hash":m.digest(policy),"phase":"OBSERVE_FIRST","trial_outcome":"not_started","restoration_outcome":"not_required","operation_attempts":{}})
                marker=root/"unsafe-import"
                if condition=="missing-original":helper.unlink()
                else:
                    helper.chmod(0o700);helper.write_text("from pathlib import Path\nPath("+repr(str(marker))+").touch()\nraise RuntimeError('unverified import')\n")
                if condition=="corrupt-stage":
                    staged=run/"tools/support";staged.chmod(0o700);staged.write_text("raise RuntimeError('corrupt')\n");staged.chmod(0o500)
                child=subprocess.run([sys.executable,"-c",launcher,str(supervisor),str(run)],capture_output=True,text=True,timeout=5)
                self.assertEqual(child.returncode,2,child.stderr)
                self.assertFalse(marker.exists(),"original helper must never be imported")
                state=json.loads((run/"state.json").read_text())
                self.assertEqual(state["phase"],"NO_ACTION")
                if condition=="corrupt-stage":self.assertIn("identity",state["reason"])
                else:self.assertIn("interrupted qualification",state["reason"])
                self.assertFalse(list(run.glob("*.stdout")),"qualification/tools must not repeat")

    @unittest.skipUnless(os.geteuid()==0,"protected freeze requires operator account")
    def test_actual_freeze_uses_one_clock_value_and_keeps_strict_bound(self):
        fixture=base.WorkflowTests();fixture.setUp()
        try:
            root=fixture.d;run=root/"freeze";tools=root/"tools.json"
            artifact=root/"artifact";artifact.write_text("isolated artifact");artifact.chmod(0o500)
            tools.write_text(json.dumps({name:str(base.SUPPORT if name=="support" else artifact) for name in m.TOOLS}))
            clock=[fixture.c.now()]
            def advancing():
                value=clock[0];clock[0]+=dt.timedelta(microseconds=1);return value
            original_lock=m.lock
            def local_lock(path,shared=False):return original_lock(root/Path(path).name,shared)
            args=["pilot","--freeze","--directory",str(run),"--panel",fixture.p["panel"],
                  "--suspects",",".join(fixture.p["suspects"]),"--controls",",".join(fixture.p["controls"]),"--tools",str(tools)]
            with patch.object(sys,"argv",args),patch.object(m,"now",advancing),patch.object(m,"lock",local_lock),patch.object(m.Adapter,"snapshot",return_value=copy.deepcopy(fixture.p["before"])),patch.object(m.Adapter,"runtime"),patch("builtins.print"):
                self.assertEqual(m.main(),0)
            policy=json.loads((run/"policy.json").read_text())
            self.assertEqual((m.instant(policy["not_after"])-m.instant(policy["created_at"])).total_seconds(),1200)
            m.validate_policy(policy)
            policy["not_after"]=(m.instant(policy["not_after"])+dt.timedelta(microseconds=1)).isoformat()
            with self.assertRaises(m.Gate):m.validate_policy(policy)
        finally:fixture.tearDown()
