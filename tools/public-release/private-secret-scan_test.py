#!/usr/bin/env python3
"""Synthetic scan-boundary tests; no live configuration or database is read."""
import importlib.util
import os
from pathlib import Path
import tempfile
import sys
import unittest
from unittest.mock import patch

sys.dont_write_bytecode = True

spec = importlib.util.spec_from_file_location(
    "private_secret_scan", Path(__file__).with_name("private-secret-scan.py"))
scanner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(scanner)

class CandidateBoundaryTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

    def reject_before_configuration(self, candidate):
        with patch.object(scanner.runpy, "run_path") as read:
            with self.assertRaises(RuntimeError):
                scanner.scan(candidate, self.root / "bootstrap.py", self.root / "configuration")
            read.assert_not_called()

    def test_missing_root(self):
        self.reject_before_configuration(self.root / "missing")

    def test_regular_file_root(self):
        target = self.root / "candidate"
        target.write_text("fixture")
        self.reject_before_configuration(target)

    def test_empty_directory_and_zero_byte_tree(self):
        self.reject_before_configuration(self.root)
        (self.root / "nested").mkdir()
        (self.root / "nested" / "empty").touch()
        self.reject_before_configuration(self.root)

    def test_git_metadata_does_not_make_export_nonempty(self):
        (self.root / ".git").mkdir()
        (self.root / ".git" / "HEAD").write_text("fixture")
        self.reject_before_configuration(self.root)

    def test_symlink_root_and_nested_file_are_refused(self):
        target = self.root / "actual"
        target.mkdir()
        (target / "safe.txt").write_text("synthetic")
        link = self.root / "link"
        link.symlink_to(target, target_is_directory=True)
        self.reject_before_configuration(link)
        link.unlink()
        (target / "outside.txt").symlink_to(self.root / "missing")
        self.reject_before_configuration(target)

    def test_nested_directory_link_and_special_file_are_refused(self):
        candidate = self.root / "candidate"
        candidate.mkdir()
        (candidate / "safe.txt").write_text("synthetic")
        alias = candidate / "alias"
        alias.symlink_to(self.root, target_is_directory=True)
        self.reject_before_configuration(candidate)
        alias.unlink()
        os.mkfifo(candidate / "fifo")
        self.reject_before_configuration(candidate)

    def test_enumeration_error_is_not_silent(self):
        def broken_walk(*args, **kwargs):
            kwargs["onerror"](PermissionError("synthetic permission failure"))
            return iter(())
        with patch.object(scanner.os, "walk", broken_walk):
            self.reject_before_configuration(self.root)

    def test_nonempty_tree_counts_regular_files_and_excludes_git(self):
        (self.root / "nested").mkdir()
        content = self.root / "nested" / "payload.txt"
        content.write_text("synthetic payload")
        empty = self.root / "empty"
        empty.touch()
        (self.root / ".git").mkdir()
        (self.root / ".git" / "HEAD").write_text("excluded")
        self.assertEqual(scanner.candidate_files(self.root), sorted([content, empty]))

if __name__ == "__main__":
    unittest.main()
