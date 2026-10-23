#!/usr/bin/env python3
import importlib.util
import json
import os
import pathlib
import subprocess
import tempfile
import time
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("updater", pathlib.Path(__file__).with_name("project-update.py"))
u = importlib.util.module_from_spec(spec)
spec.loader.exec_module(u)


class UpdateControl(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = pathlib.Path(self.tmp.name)
        self.patches = [
            patch.object(u, "STATE", self.root / "state"),
            patch.object(u, "RUNTIME", self.root / "runtime"),
            patch.object(u, "PENDING", self.root / "pending"),
            patch.object(u, "INSTALLER_LOCK", str(self.root / "installer.lock")),
            patch.object(u, "revision", return_value="1" * 40),
            patch.object(u, "unit_active", return_value=False),
        ]
        for p in self.patches:
            p.start()
        u.prepare()

    def tearDown(self):
        for p in reversed(self.patches):
            p.stop()
        self.tmp.cleanup()

    def test_allowlist_rejects_commands_urls_and_duplicate_keys(self):
        with patch.object(u.subprocess, "run") as run:
            for request in ({}, {"action": "shell"}, {"action": "start", "url": "untrusted"},
                            {"action": ["start"]}, ["start"], {"action": "status", "args": []}):
                with self.assertRaises(ValueError):
                    u.dispatch(request)
            run.assert_not_called()
        with self.assertRaises(ValueError):
            json.loads('{"action":"status","action":"start"}', object_pairs_hook=u.strict_object)

    def test_duplicate_click_attaches_to_same_job(self):
        with patch.object(u.subprocess, "run") as run:
            one = u.dispatch({"action": "start"})
            two = u.dispatch({"action": "start"})
        self.assertEqual(one["job_id"], two["job_id"])
        self.assertTrue(two["busy"])
        self.assertEqual(run.call_count, 1)
        self.assertEqual(run.call_args.args[0], ["systemctl", "start", "--no-block", u.UPDATE_UNIT])

    def test_pending_recovery_blocks_start(self):
        u.PENDING.touch()
        with self.assertRaisesRegex(ValueError, "recovery_required"):
            u.dispatch({"action": "start"})

    def test_unknown_dispatch_does_not_repeat_start(self):
        with patch.object(u.subprocess, "run", side_effect=subprocess.TimeoutExpired("systemctl", 5)) as run:
            state = u.dispatch({"action": "start"})
            self.assertEqual(state["status"], "queued")
            self.assertEqual(u.dispatch({"action": "start"})["job_id"], state["job_id"])
            self.assertEqual(run.call_count, 1)

    def test_stale_job_and_redaction(self):
        u.save({"status": "running", "stage": 4, "queued_epoch": time.time()-30,
                "private_log": "never-return-this", "token": "never-return-this"})
        state = u.dispatch({"action": "status"})
        self.assertEqual(state["status"], "interrupted")
        self.assertFalse(state["busy"])
        self.assertNotIn("never-return-this", json.dumps(state))
        self.assertEqual((u.STATE / "state.json").stat().st_mode & 0o777, 0o600)

    def test_runner_progress_survives_and_failure_is_not_success(self):
        class Process:
            stdout = iter(["[1/7] Check\n", "[4/7] Build\n", "private log detail\n"])
            def wait(self):
                return 1
        with patch.object(u.subprocess, "run"):
            job = u.dispatch({"action": "start"})
        with patch.object(u.subprocess, "Popen", return_value=Process()):
            with self.assertRaises(RuntimeError):
                u.run_update()
        state = u.dispatch({"action": "status"})
        self.assertEqual(state["job_id"], job["job_id"])
        self.assertEqual(state["status"], "failed")
        self.assertEqual(state["stage"], 4)
        self.assertNotIn("private log detail", json.dumps(state))

    def test_success_and_fixed_command(self):
        class Process:
            stdout = iter(["[6/7] Verify\n", "[7/7] Done\n"])
            def wait(self):
                return 0
        with patch.object(u.subprocess, "run"):
            u.dispatch({"action": "start"})
        with patch.object(u.subprocess, "Popen", return_value=Process()) as popen:
            u.run_update()
        self.assertEqual(popen.call_args.args[0], ["/usr/local/bin/datacenter-bot", "update"])
        state = u.dispatch({"action": "status"})
        self.assertEqual(state["status"], "succeeded")
        self.assertEqual(state["installed_revision"], "1" * 40)

    def test_root_instance_adapter_targets_its_own_runtime(self):
        class Process:
            stdout = iter(["[7/7] Done\n"])
            def wait(self):
                return 0
        command = ["/usr/local/sbin/dob-production", "update"]
        with patch.object(u, "UPDATE_UNIT", "dob-production-update.service"), \
             patch.object(u, "RESTART_UNIT", "dob-production-restart-api.service"), \
             patch.object(u, "UPDATE_COMMAND", command), \
             patch.object(u, "API_UNIT", "dob-production-api"), \
             patch.object(u, "DEPLOY_LOCK", str(self.root / "production.lock")), \
             patch.object(u.subprocess, "run") as run, \
             patch.object(u.subprocess, "Popen", return_value=Process()) as popen, \
             patch.object(u.time, "sleep"):
            u.dispatch({"action": "start"})
            self.assertEqual(run.call_args.args[0][-1], "dob-production-update.service")
            u.run_update()
            self.assertEqual(popen.call_args.args[0], command)
            u.dispatch({"action": "restart_api"})
            self.assertEqual(run.call_args.args[0][-1], "dob-production-restart-api.service")
            u.restart_api()
            self.assertEqual(run.call_args.args[0], ["systemctl", "restart", "dob-production-api"])
            with self.assertRaises(ValueError):
                u.dispatch({"action": "start", "command": ["different-target"]})

    def test_repeated_restart_replaces_an_active_request(self):
        with patch.object(u.subprocess, "run") as execute:
            u.dispatch({"action": "restart_api"})
            u.dispatch({"action": "restart_api"})
        self.assertEqual(execute.call_count, 2)
        self.assertTrue(all(call.args[0] == ["systemctl", "restart", "--no-block", u.RESTART_UNIT] for call in execute.call_args_list))

    def test_restart_busy_is_denied(self):
        with patch.object(u, "unit_active", return_value=True), patch.object(u.subprocess, "run") as run:
            with self.assertRaisesRegex(ValueError, "deployment_busy"):
                u.dispatch({"action": "restart_api"})
            run.assert_not_called()


if __name__ == "__main__":
    unittest.main(verbosity=2)
