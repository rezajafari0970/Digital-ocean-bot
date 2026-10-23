import importlib.util
import json
import tempfile
import time
import unittest
from pathlib import Path
from unittest.mock import patch
spec=importlib.util.spec_from_file_location("host_protection",Path(__file__).with_name("host-protection.py"))
hp=importlib.util.module_from_spec(spec);spec.loader.exec_module(hp)

class ProtectionTests(unittest.TestCase):
 def setUp(self):
  self.temp=tempfile.TemporaryDirectory();self.addCleanup(self.temp.cleanup)
  self.path=Path(self.temp.name)
  self.stats={"database":"test","oid":"1","failures":0,"last_failure":0,"stats_reset":10}
  self.now=time.time()
  self.report={"status":"ok","checked_at":self.now,"watermark":self.stats.copy()}
 def test_detects_new_corruption_and_statistics_reset(self):
  hp.eligible_backup(self.path,self.report,self.stats,self.now)
  for key,value in [("failures",1),("last_failure",99),("stats_reset",100)]:
   with self.subTest(key=key),self.assertRaisesRegex(RuntimeError,"database_integrity_changed"):
    hp.eligible_backup(self.path,self.report,dict(self.stats,**{key:value}),self.now)
 def test_failed_or_stale_integrity_does_not_publish_database(self):
  for report in [dict(self.report,status="failed"),dict(self.report,recovery_required=True),dict(self.report,checked_at=0),dict(self.report,checked_at=self.now+10)]:
   with self.subTest(report=report),self.assertRaises(RuntimeError):
    hp.eligible_backup(self.path,report,self.stats,self.now)
 def test_old_local_backup_rejected(self):
  with self.assertRaisesRegex(RuntimeError,"local_backup_stale"):
   hp.eligible_backup(self.path,dict(self.report,checked_at=self.now+31*3600),self.stats,self.now+31*3600)
 def test_atomic_status_is_private_and_replaces_complete_json(self):
  p=self.path/"status.json"
  hp.atomic_json(p,{"ok":True})
  self.assertEqual(p.stat().st_mode & 0o777,0o600)
  hp.atomic_json(p,{"ok":False,"reason":"test"})
  self.assertEqual(json.loads(p.read_text())["reason"],"test")
  self.assertEqual(len(list(self.path.iterdir())),1)
 def test_restic_error_never_echoes_credentials(self):
  class Failed:
   returncode=1;stdout="secret";stderr="password-secret"
  with patch.object(hp,"restic_env",return_value={}),patch.object(hp.subprocess,"run",return_value=Failed()):
   with self.assertRaisesRegex(RuntimeError,"^restic_operation_failed_1$"):
    hp.restic(["snapshots"])

if __name__=="__main__":unittest.main()
