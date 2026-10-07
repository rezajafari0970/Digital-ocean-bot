#!/usr/bin/env python3
import base64
import importlib.util
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("bootstrap", Path(__file__).with_name("bootstrap.py"))
bootstrap = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bootstrap)


class BootstrapTests(unittest.TestCase):
    def test_interrupted_publications_preserve_retry_identity(self):
        for artifact in ("master.key", "bootstrap.pending", "env"):
            for after in (False, True):
                with self.subTest(artifact=artifact, after=after), tempfile.TemporaryDirectory() as directory:
                    root = Path(directory)
                    replace = os.replace

                    def interrupted(source, target):
                        if Path(target).name == artifact:
                            if after:
                                replace(source, target)
                            raise OSError("injected publication interruption")
                        return replace(source, target)

                    with patch.object(bootstrap.os, "replace", interrupted):
                        with self.assertRaises(OSError):
                            bootstrap.prepare(root, "originaldb", "originaluser", os.getegid())
                    original = {p.name: p.read_bytes() for p in root.iterdir()}
                    pending = (root / "bootstrap.pending").exists()
                    bootstrap.prepare(root, "changeddb", "changeduser", os.getegid())
                    for name, data in original.items():
                        self.assertEqual((root / name).read_bytes(), data)
                    chosen = "originaldb" if pending else "changeddb"
                    self.assertIn("/" + chosen + "?", (root / "env").read_text())
                    self.assertEqual(len(base64.b64decode((root / "master.key").read_bytes())), 32)
                    self.assertEqual((root / "env").stat().st_mode & 0o777, 0o640)
                    self.assertEqual((root / "bootstrap.pending").stat().st_mode & 0o777, 0o600)

    def test_corrupt_persisted_artifacts_refuse_before_sql(self):
        for artifact, body in (("master.key", b"incomplete"), ("bootstrap.pending", b"db\n"),
                               ("bootstrap.pending", b"db\nuser\nextra\n")):
            with self.subTest(artifact=artifact), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                (root / artifact).write_bytes(body)
                with self.assertRaises(ValueError):
                    bootstrap.prepare(root, "db", "user", os.getegid())
                self.assertFalse((root / "env").exists())
                self.assertEqual((root / artifact).read_bytes(), body)

    def test_shell_syntax_rejected_as_data(self):
        for extra in ("EXTRA=x;id", "export EXTRA=x", "EXTRA=x>file", "EXTRA=$(id)", "EXTRA=x|id", "EXTRA=x&"):
            with self.subTest(extra=extra), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                bootstrap.prepare(root, "db", "user", os.getegid())
                (root / "bootstrap.pending").unlink()
                with (root / "env").open("a") as stream:
                    stream.write(extra + "\n")
                with self.assertRaises(ValueError):
                    bootstrap.prepare(root, "db", "user", os.getegid())

    def test_settled_environment_validated_without_pending(self):
        for body in ("", "DATABASE_URL=bad\n", "DATABASE_URL=$(must-not-run)\n"):
            with tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                bootstrap.prepare(root, "db", "user", os.getegid())
                (root / "bootstrap.pending").unlink()
                (root / "env").write_text(body)
                with self.assertRaises(ValueError):
                    bootstrap.prepare(root, "db", "user", os.getegid())
                self.assertEqual((root / "env").read_text(), body)

    def test_existing_credentials_never_rotated_or_sourced(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            bootstrap.prepare(root, "db", "user", os.getegid())
            before = {p.name: p.read_bytes() for p in root.iterdir()}
            bootstrap.prepare(root, "other", "other", os.getegid())
            self.assertEqual(before, {p.name: p.read_bytes() for p in root.iterdir()})
            (root / "env").write_text("DATABASE_URL=$(must-not-run)\n")
            with self.assertRaises(ValueError):
                bootstrap.prepare(root, "db", "user", os.getegid())


if __name__ == "__main__":
    unittest.main()
