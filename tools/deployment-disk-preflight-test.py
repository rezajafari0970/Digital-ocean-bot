#!/usr/bin/env python3
import importlib.util
from pathlib import Path
from types import SimpleNamespace
import tempfile
import unittest

ROOT=Path(__file__).resolve().parents[1]
spec=importlib.util.spec_from_file_location("disk_preflight",ROOT/"deploy/disk-preflight.py")
disk=importlib.util.module_from_spec(spec);spec.loader.exec_module(disk)

class DiskPreflight(unittest.TestCase):
    def fs(self,free,inodes=100000):
        return SimpleNamespace(f_bavail=free,f_frsize=1,f_files=1000000,f_favail=inodes)
    def test_low_space_rejected(self):
        with self.assertRaisesRegex(RuntimeError,"Insufficient deployment space"):
            disk.check_capacity([("/tmp",1000,10)],stat=lambda _:SimpleNamespace(st_dev=1),statvfs=lambda _:self.fs(999))
    def test_low_inodes_rejected(self):
        with self.assertRaisesRegex(RuntimeError,"Insufficient deployment space"):
            disk.check_capacity([("/tmp",1000,10)],stat=lambda _:SimpleNamespace(st_dev=1),statvfs=lambda _:self.fs(2000,9))
    def test_shared_filesystem_reserves_are_added(self):
        with self.assertRaises(RuntimeError):
            disk.check_capacity([("/tmp",600,10),("/var",600,10)],stat=lambda _:SimpleNamespace(st_dev=1),statvfs=lambda _:self.fs(1000))
    def test_independent_filesystems_keep_separate_reserves(self):
        disk.check_capacity([("/tmp",600,10),("/var",600,10)],stat=lambda p:SimpleNamespace(st_dev=str(p)),statvfs=lambda _:self.fs(1000))
    def test_persistent_data_and_links_counted_without_following(self):
        with tempfile.TemporaryDirectory() as tmp:
            p=Path(tmp);(p/"data").write_bytes(b"x"*10000);(p/"loop").symlink_to(p,target_is_directory=True)
            size=disk.tree_backup_bytes(p)
            self.assertGreater(size,10000);self.assertLess(size,20000)
    def test_guard_precedes_backup_and_stop(self):
        script=(ROOT/"deploy/unattended.sh").read_text()
        self.assertLess(script.index('disk-preflight.py" build'),script.index('apt-get'))
        guard=script.index('disk-preflight.py" publish')
        for action in ['dob_backup_runtime','DOB_PENDING=','dob_stop_runtime_readers','dob_publish_runtime']:
            self.assertLess(guard,script.index(action))

if __name__=="__main__":unittest.main()
