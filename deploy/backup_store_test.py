import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import backup_store as backups

spec = importlib.util.spec_from_file_location("integrity", Path(__file__).with_name("database-integrity.py"))
integrity = importlib.util.module_from_spec(spec)
spec.loader.exec_module(integrity)

class BackupStoreTests(unittest.TestCase):
    def make(self, root, n):
        pending = root / (".pending-" + str(n))
        pending.mkdir()
        (pending / "database.dump").write_bytes(b"verified database fixture")
        (pending / "configuration").mkdir()
        (pending / "configuration" / "master.key").write_bytes(b"private fixture")
        return backups.seal(pending, root / ("20261023T000000.%06dZ" % n))

    def test_interrupted_publication_never_displaces_good_copy(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            good = self.make(root, 1)
            with patch.object(backups.os, "rename", side_effect=OSError("disk full")):
                with self.assertRaises(OSError):
                    self.make(root, 2)
            self.assertTrue(backups.verified(good))
            self.assertFalse((root / "20261023T000000.000002Z").exists())
            self.assertEqual(backups.prune(root, 2), [])

    def test_retention_preserves_legacy_corrupt_and_incomplete(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            entries = [self.make(root, n) for n in range(1, 7)]
            (entries[-1] / "database.dump").write_bytes(b"corrupted")
            legacy = root / "20251023T000000.000001Z"
            legacy.mkdir()
            (legacy / "database.dump").write_bytes(b"legacy")
            pending = root / ".pending-interrupted"
            pending.mkdir()
            self.assertEqual(backups.prune(root, 3), [entries[1].name, entries[0].name])
            self.assertTrue(legacy.exists())
            self.assertTrue(pending.exists())
            self.assertTrue(entries[-1].exists())
            for entry in entries[2:5]:
                self.assertTrue(backups.verified(entry))

    def test_seal_rejects_symlink_and_empty_database(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            pending = root / ".pending"
            pending.mkdir()
            target = root / "20261023T000000.000001Z"
            with self.assertRaises(ValueError):
                backups.seal(pending, target)
            (pending / "database.dump").symlink_to("/etc/passwd")
            with self.assertRaises(ValueError):
                backups.seal(pending, target)
            self.assertFalse(target.exists())

class IntegrityTests(unittest.TestCase):
    def test_new_failure_or_statistics_reset_requires_explicit_recovery(self):
        stats = {"database": "test", "oid": "42", "failures": 2920, "last_failure": 100, "stats_reset": 10}
        previous = {"watermark": dict(stats), "recovery_required": False}
        self.assertFalse(integrity.needs_acknowledgement(previous, stats))
        self.assertTrue(integrity.needs_acknowledgement(None, stats))
        for key in ("failures", "last_failure", "stats_reset"):
            changed = dict(stats)
            changed[key] += 1
            self.assertTrue(integrity.needs_acknowledgement(previous, changed))
        previous["recovery_required"] = True
        self.assertTrue(integrity.needs_acknowledgement(previous, stats))

    def test_transient_failure_does_not_force_manual_repair_on_clean_database(self):
        stats = {"database": "test", "oid": "42", "failures": 0, "last_failure": 0, "stats_reset": 10}
        self.assertFalse(integrity.needs_acknowledgement(None, stats))
        self.assertFalse(integrity.needs_acknowledgement({"watermark": None, "recovery_required": False}, stats))

    def test_failed_check_never_acknowledges_corruption(self):
        stats = {"database": "test", "oid": "42", "failures": 1, "last_failure": 100, "stats_reset": 10,
                 "checksums": "on", "fsync": "on", "full_page_writes": "on", "zero_damaged_pages": "off",
                 "ignore_checksum_failure": "off", "synchronous_commit": "on"}
        with tempfile.TemporaryDirectory() as directory, patch.object(integrity, "STATE", Path(directory)):
            with patch.object(integrity, "query", side_effect=[json.dumps(stats), "180006"]), patch.object(Path, "is_file", return_value=True):
                with patch.object(integrity.subprocess, "run") as run:
                    run.return_value.returncode = 1
                    with self.assertRaisesRegex(RuntimeError, "integrity_check_failed"):
                        integrity.inspect({"PGHOST": "127.0.0.1", "PGDATABASE": "test"}, None, True)

spec = importlib.util.spec_from_file_location("runtime_retention", Path(__file__).with_name("runtime-retention.py"))
runtime = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runtime)

class RuntimeRetentionTests(unittest.TestCase):
    def test_only_verified_completed_rollbacks_expire(self):
        import hashlib
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            paths = []
            for n in range(4):
                path = root / ("digital-ocean-bot.rollback.fixture%d" % n)
                (path / "runtime/bin").mkdir(parents=True)
                body = b"runtime fixture"
                for file in ("digital-ocean-bot-api", "digital-ocean-bot-worker"):
                    (path / "runtime/bin" / file).write_bytes(body)
                sha = hashlib.sha256(body).hexdigest()
                (path / "runtime/build-manifest.json").write_text(json.dumps({"commit": "a"*40, "api_sha256": sha, "worker_sha256": sha}))
                paths.append(path)
            # Unmarked rollback evidence is never automatically discarded.
            for path in paths[1:]:
                runtime.retain(path, root)
            self.assertTrue(paths[0].exists())
            self.assertFalse(paths[1].exists())
            self.assertTrue(paths[2].exists())
            self.assertTrue(paths[3].exists())
            (paths[3] / "runtime/bin/digital-ocean-bot-api").write_bytes(b"corrupt")
            with self.assertRaises(ValueError):
                runtime.identity(paths[3])


class IntegritySafetyRegressionTests(unittest.TestCase):
    def stats(self):
        return {"database": "fixture", "oid": "42", "failures": 0, "last_failure": 0,
                "stats_reset": 0, "checksums": "on", "fsync": "on", "full_page_writes": "on",
                "synchronous_commit": "on", "zero_damaged_pages": "off", "ignore_checksum_failure": "off"}

    def test_disabled_or_missing_checksums_never_report_healthy(self):
        for value in ("off", None):
            with self.subTest(checksums=value), tempfile.TemporaryDirectory() as directory:
                stats = self.stats()
                if value is None:
                    del stats["checksums"]
                else:
                    stats["checksums"] = value
                with patch.object(integrity, "STATE", Path(directory)), patch.object(integrity, "query", side_effect=[json.dumps(stats), "180006", json.dumps(stats)]), patch.object(Path, "is_file", return_value=True), patch.object(integrity.subprocess, "run") as run:
                    run.return_value.returncode = 0
                    with self.assertRaisesRegex(RuntimeError, "unsafe_database_settings"):
                        integrity.inspect({"PGHOST": "127.0.0.1", "PGDATABASE": "fixture"}, None, True)
                    run.assert_not_called()

    def test_setting_change_during_inspection_is_rejected(self):
        for key, value in (("checksums", "off"), ("fsync", "off"), ("full_page_writes", "off"),
                           ("synchronous_commit", "off"), ("zero_damaged_pages", "on"),
                           ("ignore_checksum_failure", "on")):
            with self.subTest(setting=key), tempfile.TemporaryDirectory() as directory:
                before = self.stats()
                after = dict(before, **{key: value})
                with patch.object(integrity, "STATE", Path(directory)), patch.object(integrity, "query", side_effect=[json.dumps(before), "180006", json.dumps(after)]), patch.object(Path, "is_file", return_value=True), patch.object(integrity.subprocess, "run") as run:
                    run.return_value.returncode = 0
                    with self.assertRaisesRegex(RuntimeError, "unsafe_database_settings"):
                        integrity.inspect({"PGHOST": "127.0.0.1", "PGDATABASE": "fixture"}, None)

    def test_healthy_checks_still_pass(self):
        stats = self.stats()
        with tempfile.TemporaryDirectory() as directory, patch.object(integrity, "STATE", Path(directory)), patch.object(integrity, "query", side_effect=[json.dumps(stats), "180006", json.dumps(stats)]), patch.object(Path, "is_file", return_value=True), patch.object(integrity.subprocess, "run") as run:
            run.return_value.returncode = 0
            self.assertEqual(integrity.inspect({"PGHOST": "127.0.0.1", "PGDATABASE": "fixture"}, None)["status"], "ok")


class BackupFreshnessRegressionTests(unittest.TestCase):
    def make(self, root, name):
        pending = root / (".pending-" + name)
        pending.mkdir()
        (pending / "database.dump").write_bytes(b"fixture dump")
        return backups.seal(pending, root / name)

    def test_directory_mtime_cannot_refresh_old_backup(self):
        import datetime
        current = datetime.datetime(2026, 10, 25, 12, tzinfo=datetime.timezone.utc)
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            old = self.make(root, "20261024T000000.000001Z")
            os.utime(old, (current.timestamp(), current.timestamp()))
            self.assertFalse(integrity.has_recent_verified_backup(root, current))

    def test_only_a_fresh_verified_generation_suppresses_new_dump(self):
        import datetime
        current = datetime.datetime(2026, 10, 25, 12, tzinfo=datetime.timezone.utc)
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            fresh = self.make(root, "20261025T100000.000001Z")
            self.assertTrue(integrity.has_recent_verified_backup(root, current))
            (fresh / "database.dump").write_bytes(b"damaged fixture")
            self.assertFalse(integrity.has_recent_verified_backup(root, current))
            future = self.make(root, "20261026T100000.000001Z")
            self.assertFalse(integrity.has_recent_verified_backup(root, current))
            (root / "20261025T110000.000001Z").symlink_to(future, target_is_directory=True)
            self.assertFalse(integrity.has_recent_verified_backup(root, current))
