#!/usr/bin/env python3
import json
from pathlib import Path
import runpy
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


if __name__ == '__main__':
    unittest.main()
