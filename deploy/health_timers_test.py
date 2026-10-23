#!/usr/bin/env python3
"""Exercise the shipped health timer with real systemd and disposable no-op units.

Run as root on a systemd host. The release smoke gate runs this in its isolated
container. Production timer/service files and database contents are never changed.
"""
import os
from pathlib import Path
import subprocess
import tempfile
import time
import unittest
import uuid

SOURCE = Path(__file__).resolve().parent

def command(*args):
    return subprocess.check_output(args, text=True, stderr=subprocess.STDOUT).strip()

class IntegrityTimerTests(unittest.TestCase):
    def setUp(self):
        self.assertEqual(os.geteuid(), 0, "real systemd regression requires root")
        self.name = "dob-health-timer-test-" + uuid.uuid4().hex
        self.root = Path(tempfile.mkdtemp(prefix=self.name + "-", dir="/run"))
        self.events = self.root / "events"
        self.timer = Path("/run/systemd/system") / (self.name + ".timer")
        self.service = self.timer.with_suffix(".service")
        self.stamp = Path("/var/lib/systemd/timers") / ("stamp-" + self.name + ".timer")
        self.addCleanup(self.cleanup)
        self.service.write_text(
            "[Unit]\nDescription=Disposable health timer regression\n"
            "[Service]\nType=oneshot\nExecStart=/usr/bin/tee -a " + str(self.events) +
            "\nStandardInput=data\nStandardInputText=tick\nStandardOutput=null\n")
        self.body = (SOURCE / "datacenter-bot-integrity.timer").read_text()

    def cleanup(self):
        subprocess.run(["systemctl", "stop", self.name + ".timer", self.name + ".service"],
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        for path in (self.timer, self.service, self.stamp, self.events):
            path.unlink(missing_ok=True)
        self.root.rmdir()
        command("systemctl", "daemon-reload")
        subprocess.run(["systemctl", "reset-failed", self.name + ".service"],
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

    def install(self, body):
        # Only jitter/accuracy are accelerated. Unit ownership is the disposable
        # filename; the shipped scheduling policy is otherwise retained.
        body = body.replace("RandomizedDelaySec=5min", "RandomizedDelaySec=0")
        self.timer.write_text(body.replace("[Timer]\n", "[Timer]\nAccuracySec=10ms\n"))
        self.stamp.touch()
        past = time.time() - 7 * 3600
        os.utime(self.stamp, (past, past))
        command("systemctl", "daemon-reload")
        command("systemctl", "start", self.name + ".timer")

    def properties(self):
        return dict(line.split("=", 1) for line in command(
            "systemctl", "show", self.name + ".timer",
            "-p", "ActiveState", "-p", "SubState", "-p", "NextElapseUSecRealtime",
            "-p", "NextElapseUSecMonotonic").splitlines())

    def assertScheduled(self):
        end = time.monotonic() + 3
        while True:
            props = self.properties()
            finite = any(props.get(key, "") not in ("", "0", "infinity", "n/a")
                         for key in ("NextElapseUSecRealtime", "NextElapseUSecMonotonic"))
            if props.get("ActiveState") == "active" and props.get("SubState") == "waiting" and finite:
                return
            if time.monotonic() >= end:
                self.fail("Timer has no next inspection: " + repr(props))
            time.sleep(0.1)

    def count(self):
        return len(self.events.read_text().splitlines()) if self.events.exists() else 0

    def awaitCount(self, expected):
        end = time.monotonic() + 12
        while self.count() < expected and time.monotonic() < end:
            time.sleep(0.1)
        self.assertGreaterEqual(self.count(), expected, "timer did not execute repeatedly")

    def test_previous_boot_stamp_cannot_leave_timer_elapsed(self):
        self.install(self.body)
        self.assertScheduled()
        self.awaitCount(1)
        command("systemctl", "restart", self.name + ".timer")
        self.assertScheduled()

    def test_repeated_execution_survives_stop_and_start(self):
        # Speed up only the recurring interval; keep the actual shipped policy.
        body = self.body.replace("00/6:00:00", "*:*:0/2").replace("OnUnitActiveSec=6h", "OnUnitActiveSec=2s")
        self.install(body)
        self.awaitCount(3)
        command("systemctl", "stop", self.name + ".timer")
        before = self.count()
        command("systemctl", "start", self.name + ".timer")
        self.awaitCount(before + 2)
        self.assertScheduled()

if __name__ == "__main__":
    unittest.main()
