import importlib.util,json,pathlib,tempfile,unittest
from unittest.mock import patch
spec=importlib.util.spec_from_file_location("instance_update",pathlib.Path(__file__).with_name("instance-update.py"))
u=importlib.util.module_from_spec(spec);spec.loader.exec_module(u)
class TransactionTests(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.root=pathlib.Path(self.tmp.name)
  self.live=self.root/"live";self.live.mkdir();(self.live/"old").write_text("original")
  self.candidate=self.root/"candidate";self.candidate.mkdir();(self.candidate/"new").write_text("candidate")
  self.etc=self.root/"etc";self.etc.mkdir()
  for name in ["env","master.key"]:(self.etc/name).write_text("fixture")
  self.identity={"source_commit":"1"*40};(self.etc/"instance.json").write_text(json.dumps(self.identity))
  self.health=self.root/"health";self.health.write_text(json.dumps(dict(status="ok",recovery_required=False,backup=dict(ok=True))))
  self.state=self.root/"state";self.state.mkdir()
  self.patches=[patch.object(u,"ROOT",self.live),patch.object(u,"ETC",self.etc),patch.object(u,"PENDING",self.etc/"pending"),patch.object(u,"STATE",self.state),patch.object(u,"HEALTH",self.health),
   patch.object(u,"manifest",return_value={"commit":"1"*40}),patch.object(u,"verify_runtime",return_value={"commit":"2"*40}),
   patch.object(u.runpy,"run_path",return_value={"read_environment":lambda p:{},"database_environment":lambda e:{}}),
   patch.object(u.subprocess,"check_output",side_effect=lambda args,**kw: "100" if args[0]=="psql" else "0"),
   patch.object(u.shutil,"disk_usage",return_value=type("Space",(),{"free":10**12})())]
  for p in self.patches:p.start()
  # Map only the installer's fixed rollback/failed paths into the test directory.
  self.original_path=u.pathlib.Path
  self.path_patch=patch.object(u.pathlib,"Path",side_effect=lambda p,*a:self.root/pathlib.PurePath(p).name if str(p).startswith("/opt/dob-production.") else self.original_path(p,*a))
  self.path_patch.start()
 def tearDown(self):
  self.path_patch.stop()
  for p in reversed(self.patches):p.stop()
  self.tmp.cleanup()
 def test_migration_failure_restores_runtime_identity_and_readiness(self):
  with patch.object(u,"run") as run,patch.object(u,"migrate",side_effect=RuntimeError("fixture_failure")),patch.object(u,"ready") as ready:
   with self.assertRaisesRegex(RuntimeError,"fixture_failure"):u.deploy(self.candidate)
  self.assertTrue((self.live/"old").exists());self.assertFalse((self.live/"new").exists())
  self.assertEqual(json.loads((self.etc/"instance.json").read_text()),self.identity)
  self.assertFalse(u.PENDING.exists());ready.assert_called_once_with("1"*40)
  self.assertTrue(json.loads((self.state/"deployment-result.json").read_text())["rollback_ready"])
  self.assertFalse(any("digital-ocean-bot-api" in str(c) for c in run.call_args_list))
 def test_unhealthy_rollback_keeps_durable_recovery_marker(self):
  with patch.object(u,"run"),patch.object(u,"migrate",side_effect=RuntimeError("fixture_failure")),patch.object(u,"ready",side_effect=RuntimeError("not_ready")):
   with self.assertRaisesRegex(RuntimeError,"not_ready"):u.deploy(self.candidate)
  self.assertTrue((self.live/"old").exists())
  self.assertEqual(json.loads(u.PENDING.read_text())["phase"],"rolling_back")
 def test_success_keeps_previous_runtime_and_verifies_new_identity(self):
  with patch.object(u,"run"),patch.object(u,"migrate"),patch.object(u,"ready") as ready:u.deploy(self.candidate)
  self.assertTrue((self.live/"new").exists());ready.assert_called_once_with("2"*40)
  self.assertEqual(json.loads((self.etc/"instance.json").read_text())["source_commit"],"2"*40)
  self.assertFalse(u.PENDING.exists())
  self.assertTrue(any((p/"old").exists() for p in self.root.glob("dob-production.rollback-*")))
 def test_recovery_required_never_stops_services(self):
  self.health.write_text(json.dumps(dict(status="corrupt",recovery_required=True)))
  with patch.object(u,"run") as run:
   with self.assertRaisesRegex(RuntimeError,"integrity_recovery_required"):u.deploy(self.candidate)
  self.assertTrue((self.live/"old").exists());self.assertFalse(u.PENDING.exists())
  self.assertFalse(any(c.args[0][:2]==["systemctl","stop"] for c in run.call_args_list))
if __name__=="__main__":unittest.main(verbosity=2)
