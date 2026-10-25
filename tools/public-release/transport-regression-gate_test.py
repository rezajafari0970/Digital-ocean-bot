#!/usr/bin/env python3
import json
from pathlib import Path
import runpy
import shutil
import subprocess
import sys
import tempfile
import unittest

verify = runpy.run_path(str(Path(__file__).with_name('transport-regression-gate.py')))['verify_events']
REQUIRED = {'internal/adminapi': ('TestDeletion', 'TestCleanupFence')}


def event(action, test='', package='example/project/internal/adminapi'):
    return json.dumps({'Action': action, 'Test': test, 'Package': package})


class ReleaseEvidenceTests(unittest.TestCase):
    def test_all_required_tests_must_pass(self):
        self.assertEqual(verify([event('pass', 'TestDeletion'), event('pass', 'TestCleanupFence')], REQUIRED), 2)

    def test_successful_package_without_tests_is_rejected(self):
        with self.assertRaisesRegex(RuntimeError, 'did not pass'):
            verify([event('pass')], REQUIRED)

    def test_missing_or_renamed_regression_is_rejected(self):
        with self.assertRaisesRegex(RuntimeError, 'TestCleanupFence'):
            verify([event('pass', 'TestDeletion'), event('pass', 'TestRenamed')], REQUIRED)

    def test_skipped_database_fixture_is_rejected(self):
        with self.assertRaisesRegex(RuntimeError, 'skipped'):
            verify([event('skip', 'TestDeletion')], REQUIRED)

    def test_skipped_subtest_is_rejected_even_when_parent_passes(self):
        with self.assertRaisesRegex(RuntimeError, 'skipped'):
            verify([event('pass', 'TestDeletion'), event('skip', 'TestCleanupFence/PAUSED'), event('pass', 'TestCleanupFence')], REQUIRED)

    def test_failing_package_or_test_is_rejected(self):
        for test in ('', 'TestDeletion'):
            with self.subTest(test=test), self.assertRaisesRegex(RuntimeError, 'failed'):
                verify([event('fail', test)], REQUIRED)

    def test_malformed_evidence_is_rejected(self):
        with self.assertRaisesRegex(RuntimeError, 'invalid'):
            verify(['not a test event'], REQUIRED)


class GateEntrypointTests(unittest.TestCase):
    repository_gate = Path(__file__).resolve().parents[2] / 'ops/release/transport-regression-gate.py'
    publisher_gate = Path(__file__).with_name('transport-regression-gate.py').resolve()

    def test_publisher_must_enforce_repository_policy(self):
        repository = runpy.run_path(str(self.repository_gate))
        publisher = runpy.run_path(str(self.publisher_gate))
        self.assertEqual(repository['REQUIRED'], publisher['REQUIRED'])

    def test_publisher_gate_survives_credential_free_copy(self):
        repository = runpy.run_path(str(self.repository_gate))
        with tempfile.TemporaryDirectory() as directory:
            copied = Path(directory) / 'gate.py'
            shutil.copyfile(self.publisher_gate, copied)
            standalone = runpy.run_path(str(copied))
            self.assertEqual(repository['REQUIRED'], standalone['REQUIRED'])
            expected = sum(len(names) for names in repository['REQUIRED'].values())
            events = [event('pass', name, 'example/project/' + package)
                      for package, names in repository['REQUIRED'].items() for name in names]
            self.assertEqual(standalone['verify_events'](events), expected)
            with self.assertRaisesRegex(RuntimeError, 'did not pass'):
                standalone['verify_events'](events[:-1])

    def test_repository_entrypoint_preserves_fail_closed_cli(self):
        results = [subprocess.run([sys.executable, '-I', str(path)],
                                  capture_output=True, text=True, timeout=10)
                   for path in (self.repository_gate, self.publisher_gate)]
        self.assertEqual([result.returncode for result in results], [1, 1])
        self.assertEqual(results[0].stderr, results[1].stderr)
        self.assertIn('Transport regression gate blocked: usage:', results[0].stderr)


if __name__ == '__main__':
    unittest.main()
