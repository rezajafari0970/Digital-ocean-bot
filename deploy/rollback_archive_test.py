import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import time
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location("archive_driver",Path(__file__).with_name("rollback-archive.py"))
driver=importlib.util.module_from_spec(spec)
spec.loader.exec_module(driver)

class ArchiveTests(unittest.TestCase):
    def fixture(self,root,name="dob-production.rollback-20261001T000000"):
        p=root/name
        (p/"bin").mkdir(parents=True)
        raw=b"fixture reproducible binary\n"*4000
        for f in ("digital-ocean-bot-api","digital-ocean-bot-worker"):
            (p/"bin"/f).write_bytes(raw)
            (p/"bin"/f).chmod(0o755)
        (p/"build-manifest.json").write_text(json.dumps({"commit":"a"*40,"api_sha256":hashlib.sha256(raw).hexdigest(),"worker_sha256":hashlib.sha256(raw).hexdigest()}))
        return p
    def test_roundtrip_and_compression(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d);p=self.fixture(root);before=driver.inventory(p)
            saved=driver.archive_one(p,root/"archives",lambda p:False)
            self.assertFalse(p.exists())
            self.assertFalse(p.with_name(p.name+driver.PENDING).exists())
            driver.verify_archive(Path(saved["archive"]),before)
            self.assertLess(saved["archive_bytes"],saved["source_bytes"]/2)
            self.assertEqual(saved,json.loads((root/(p.name+".archived.json")).read_text()))
    def test_current_foreign_link_and_in_use_are_preserved(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d)
            for name in ("dob-production","another-project.rollback-20261001T000000"):
                p=self.fixture(root,name)
                with self.assertRaises(ValueError):driver.archive_one(p,root/"archives",lambda p:False)
                self.assertTrue(p.exists())
            p=self.fixture(root)
            with self.assertRaises(ValueError):driver.archive_one(p,root/"archives",lambda p:True)
            (p/"foreign").symlink_to(root/"dob-production")
            with self.assertRaises(ValueError):driver.archive_one(p,root/"archives",lambda p:False)
            self.assertTrue(p.exists())
    def test_keep_three_and_recent_versions(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d)
            for day in range(1,6):self.fixture(root,f"dob-production.rollback-2026100{day}T000000")
            names=[p.name for p in driver.candidates(root,time.time())]
            self.assertEqual(names,["dob-production.rollback-20261001T000000","dob-production.rollback-20261002T000000"])
            self.assertEqual(driver.candidates(root,0),[])
    def test_interrupted_removal_resumes_verified_archive(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d);p=self.fixture(root)
            with patch.object(driver.shutil,"rmtree",side_effect=OSError("interrupted")):
                with self.assertRaises(OSError):driver.archive_one(p,root/"archives",lambda p:False)
            pending=root/(p.name+driver.PENDING)
            self.assertTrue(pending.exists())
            (pending/"bin/digital-ocean-bot-api").unlink()
            driver.archive_one(pending,root/"archives",lambda p:False)
            self.assertFalse(pending.exists())
    def test_corrupt_archive_or_changed_pending_never_removes(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d);p=self.fixture(root)
            with patch.object(driver.shutil,"rmtree",side_effect=OSError("interrupted")):
                with self.assertRaises(OSError):driver.archive_one(p,root/"archives",lambda p:False)
            pending=root/(p.name+driver.PENDING)
            (pending/"unexpected").write_text("preserve")
            with self.assertRaises(ValueError):driver.archive_one(pending,root/"archives",lambda p:False)
            self.assertTrue(pending.exists())
            target=root/"archives"/(p.name+".tar.gz")
            target.write_bytes(b"corrupt")
            with self.assertRaises(ValueError):driver.archive_one(pending,root/"archives",lambda p:False)
            self.assertTrue(pending.exists())

if __name__=="__main__":unittest.main()
