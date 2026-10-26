"""Budget, crash-recovery and no-resubmission contracts; no external API calls."""
import copy
import fcntl
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("bounded_review", Path(__file__).with_name("bounded-openai-review.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class BoundedReviewTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.ledger = Path(self.directory.name) / "ledger.json"
        self.posts = 0
        self.gets = 0

    def state(self):
        return json.loads(self.ledger.read_text())

    def save(self, state):
        module.atomic_write(self.ledger, state)

    def response(self, status="completed", **changes):
        result = {"id": "resp_fixture", "model": module.MODEL, "status": status,
                  "service_tier": "default", "usage": {"input_tokens": 100, "output_tokens": 20},
                  "output": [{"content": [{"type": "output_text", "text": "review findings"}]}]}
        if status in module.PENDING:
            result["usage"] = None
            result["output"] = []
        result.update(changes)
        return result

    def send(self, payload, key):
        self.posts += 1
        self.assertEqual(key, "fake-test-key")
        self.assertEqual(payload["model"], "gpt-6-astra")
        self.assertEqual(payload["reasoning"], {"effort": "max"})
        self.assertTrue(payload["background"])
        self.assertTrue(payload["store"])
        self.assertEqual(payload["service_tier"], "default")
        self.assertEqual(payload["prompt_cache_options"], {"ttl": "30m"})
        row = self.state()["requests"][-1]
        self.assertEqual(row["status"], "reserved")
        self.assertEqual(row["reserved_micro_usd"], row["charged_micro_usd"])
        self.assertEqual(self.ledger.stat().st_mode & 0o777, 0o600)
        return self.response()

    def submit(self, prompt="review public code", **kwargs):
        kwargs.setdefault("send", self.send)
        kwargs.setdefault("label", "review-" + str(self.posts))
        return module.submit_review(prompt, "fake-test-key", self.ledger, **kwargs)

    def queue(self, prompt="review public code"):
        def send(payload, key):
            self.send(payload, key)
            return self.response("queued")
        return self.submit(prompt, send=send)

    def fetch(self, response_id, key):
        self.gets += 1
        self.assertEqual(response_id, "resp_fixture")
        self.assertEqual(key, "fake-test-key")
        return self.response()

    def legacy(self, status="unknown"):
        reserve = 1501011
        row = {"attempt": "497a22e2-ff8b-49e5-bc97-f9162df86e5a", "at": 1,
               "payload_digest": module.digest(module.payload_for("old review", module.MAX_OUTPUT, background=False)),
               "status": status, "reserved_micro_usd": reserve, "charged_micro_usd": reserve}
        if status == "completed":
            row.update(usage={"input_tokens": 100, "output_tokens": 20}, charged_micro_usd=2300)
        return {"schema": 1, "policy": module.POLICY.copy(), "requests": [row]}

    def test_cli_submit_reaches_existing_budget_guard(self):
        import io
        with patch.object(module.sys, "argv", ["bounded-openai-review.py", "submit", "--label", "udp-review", "--max-output-tokens", "512"]), patch.object(module.sys, "stdin", io.StringIO("public review")), patch.object(module.sys, "stderr", io.StringIO()) as error, patch.object(module, "submit_review", side_effect=module.ReviewBlocked("unresolved_request_requires_usage_reconciliation")) as submit:
            self.assertEqual(module.main(), 78)
            self.assertEqual(submit.call_args.kwargs, {"max_output_tokens": 512, "label": "udp-review", "independent_review": False, "reasoning_effort": "max"})
            self.assertIn("unresolved_request_requires_usage_reconciliation", error.getvalue())

    def held_unknown(self):
        self.save(self.legacy())
        module.migrate_ledger(self.ledger)
        return copy.deepcopy(self.state()["requests"][0])

    def test_independent_review_preserves_unknown_and_total_cap(self):
        old = self.held_unknown()
        result = self.submit("new independent DNS review", independent_review=True, max_output_tokens=512)
        state = self.state()
        self.assertEqual(state["requests"][0], old)
        self.assertEqual(state["requests"][1]["independent_prior_unknowns"], [old["attempt"]])
        self.assertLessEqual(sum(x["charged_micro_usd"] for x in state["requests"]), module.BUDGET_MICRO)
        self.assertEqual((self.posts, result["status"]), (1, "settled"))

    def test_independent_review_cannot_replay_legacy_or_background_prompt(self):
        old = self.held_unknown()
        with self.assertRaisesRegex(module.ReviewBlocked, "duplicate"):
            self.submit("old review", independent_review=True, max_output_tokens=512)
        self.submit("new distinct review", independent_review=True, max_output_tokens=512)
        with self.assertRaisesRegex(module.ReviewBlocked, "duplicate"):
            self.submit("new distinct review", independent_review=True, max_output_tokens=256, label="changed")
        self.assertEqual(self.state()["requests"][0], old)
        self.assertEqual(self.posts, 1)

    def test_independent_review_cannot_release_reservation_or_exceed_cap(self):
        old = self.held_unknown()
        with self.assertRaisesRegex(module.ReviewBlocked, "exceeds_cap"):
            self.submit("new review", independent_review=True)
        self.assertEqual(self.state()["requests"], [old])
        self.assertEqual(self.posts, 0)

    def test_independent_review_does_not_bypass_pending_receipt(self):
        self.queue()
        with self.assertRaisesRegex(module.ReviewBlocked, "already_pending"):
            self.submit("another task", independent_review=True, max_output_tokens=512)
        self.assertEqual(self.posts, 1)

    def test_focused_effort_keeps_model_and_cap(self):
        def send(payload, key):
            self.assertEqual(payload["model"], "gpt-6-astra")
            self.assertEqual(payload["reasoning"], {"effort": "low"})
            self.assertEqual(self.state()["requests"][-1]["reasoning_effort"], "low")
            return self.response()
        result = self.submit("focused new decision", reasoning_effort="low", max_output_tokens=1800, send=send)
        self.assertEqual(result["reasoning_effort"], "low")
        self.assertEqual(result["model"], "gpt-6-astra")
        with self.assertRaisesRegex(module.ReviewBlocked, "invalid"):
            self.submit("bad effort", reasoning_effort="none")

    def test_submission_is_prefunded_private_and_result_verified(self):
        result = self.submit()
        self.assertEqual(result["status"], "settled")
        self.assertAlmostEqual(result["estimated_cost_usd_upper"], 0.0023)
        self.assertEqual(module.review_result(self.ledger)["text"], "review findings")
        row = self.state()["requests"][0]
        artifact = self.ledger.parent / row["result_file"]
        self.assertEqual(artifact.stat().st_mode & 0o777, 0o600)
        self.assertEqual(self.posts, 1)

    def test_queued_receipt_survives_new_invocation_without_post(self):
        result = self.queue()
        self.assertEqual(result["status"], "pending")
        self.assertTrue(result["receipt_saved"])
        self.assertEqual(self.state()["requests"][0]["response_id"], "resp_fixture")
        # Reloading persisted state, not an in-memory response, drives each GET.
        self.assertEqual(module.poll_review("fake-test-key", self.ledger, fetch=self.fetch)["status"], "settled")
        module.poll_review("fake-test-key", self.ledger, fetch=self.fetch)
        self.assertEqual((self.posts, self.gets), (1, 1))

    def test_pending_request_blocks_new_submission(self):
        self.queue()
        with self.assertRaisesRegex(module.ReviewBlocked, "already_pending"):
            self.submit("different source")
        self.assertEqual(self.posts, 1)

    def test_poll_timeout_retains_receipt_and_full_reserve(self):
        self.queue()
        before = self.state()["requests"][0]
        def timeout(*args):
            raise TimeoutError("private diagnostic")
        with self.assertRaisesRegex(module.ReviewBlocked, "receipt_and_reservation_retained"):
            module.poll_review("fake-test-key", self.ledger, fetch=timeout)
        after = self.state()["requests"][0]
        self.assertEqual(after["response_id"], before["response_id"])
        self.assertEqual(after["charged_micro_usd"], before["reserved_micro_usd"])
        self.assertNotIn("private diagnostic", self.ledger.read_text())
        module.poll_review("fake-test-key", self.ledger, fetch=self.fetch)
        self.assertEqual((self.posts, self.gets), (1, 1))

    def test_in_progress_poll_preserves_reservation(self):
        self.queue()
        before = self.state()["requests"][0]["reserved_micro_usd"]
        result = module.poll_review("fake-test-key", self.ledger, fetch=lambda *args: self.response("in_progress"))
        self.assertEqual(result["status"], "pending")
        self.assertEqual(self.state()["requests"][0]["charged_micro_usd"], before)

    def test_lost_post_response_is_not_retried(self):
        def timeout(payload, key):
            self.send(payload, key)
            raise TimeoutError("fake-test-key must not appear")
        with self.assertRaisesRegex(module.ReviewBlocked, "uncertain"):
            self.submit(send=timeout)
        row = self.state()["requests"][0]
        self.assertEqual(row["status"], "unknown")
        self.assertEqual(row["charged_micro_usd"], row["reserved_micro_usd"])
        self.assertNotIn("fake-test-key", self.ledger.read_text())
        with self.assertRaisesRegex(module.ReviewBlocked, "duplicate"):
            self.submit(max_output_tokens=100)
        with self.assertRaisesRegex(module.ReviewBlocked, "requires_usage_reconciliation"):
            self.submit("different review")
        with self.assertRaisesRegex(module.ReviewBlocked, "no_resumable"):
            module.poll_review("fake-test-key", self.ledger, fetch=self.fetch)
        self.assertEqual((self.posts, self.gets), (1, 0))

    def test_migration_preserves_unknown_evidence_and_hard_stop(self):
        original = self.legacy()
        self.save(original)
        with self.assertRaisesRegex(module.ReviewBlocked, "policy_mismatch"):
            self.submit()
        result = module.migrate_ledger(self.ledger)
        evidence = self.ledger.with_name("ledger.schema1.evidence.json")
        self.assertEqual(json.loads(evidence.read_text()), original)
        self.assertEqual(result["unresolved_reserved_usd"], 1.501011)
        migrated = self.state()
        for key, value in original["requests"][0].items():
            self.assertEqual(migrated["requests"][0][key], value)
        module.migrate_ledger(self.ledger)
        self.assertEqual(self.state(), migrated)
        with self.assertRaisesRegex(module.ReviewBlocked, "duplicate"):
            self.submit("old review", max_output_tokens=100)
        with self.assertRaisesRegex(module.ReviewBlocked, "requires_usage_reconciliation"):
            self.submit()
        self.assertEqual(self.posts, 0)

    def test_migration_completed_usage_and_reserved_crash(self):
        for old_status, expected in [("completed", "settled"), ("reserved", "unknown")]:
            with self.subTest(old_status=old_status):
                self.save(self.legacy(old_status))
                evidence = self.ledger.with_name("ledger.schema1.evidence.json")
                if evidence.exists():
                    evidence.unlink()
                module.migrate_ledger(self.ledger)
                self.assertEqual(self.state()["requests"][0]["status"], expected)

    def test_migration_conflicting_evidence_blocks_without_changes(self):
        original = self.legacy()
        self.save(original)
        module.atomic_write(self.ledger.with_name("ledger.schema1.evidence.json"), {"different": True})
        with self.assertRaisesRegex(module.ReviewBlocked, "evidence_conflict"):
            module.migrate_ledger(self.ledger)
        self.assertEqual(self.state(), original)

    def test_durable_result_recovers_crash_before_ledger_settlement(self):
        self.queue()
        real_write = module.atomic_write
        def interrupted_write(path, data):
            if path == self.ledger:
                raise OSError("simulated storage interruption")
            real_write(path, data)
        with patch.object(module, "atomic_write", side_effect=interrupted_write):
            with self.assertRaises(OSError):
                module.poll_review("fake-test-key", self.ledger, fetch=self.fetch)
        self.assertEqual(self.state()["requests"][0]["status"], "pending")
        def no_fetch(*args):
            self.fail("durable result should be used without another network request")
        result = module.poll_review("fake-test-key", self.ledger, fetch=no_fetch)
        self.assertEqual(result["status"], "settled")
        self.assertEqual(module.review_result(self.ledger)["text"], "review findings")
        self.assertEqual((self.posts, self.gets), (1, 1))

    def test_reservation_write_failure_prevents_post(self):
        with patch.object(module, "atomic_write", side_effect=OSError("disk full")):
            with self.assertRaises(OSError):
                self.submit()
        self.assertEqual(self.posts, 0)

    def test_receipt_survives_invalid_terminal_usage_then_get_recovers(self):
        def missing_usage(payload, key):
            self.send(payload, key)
            return self.response(usage=None)
        with self.assertRaisesRegex(module.ReviewBlocked, "uncertain"):
            self.submit(send=missing_usage)
        self.assertEqual(self.state()["requests"][0]["response_id"], "resp_fixture")
        module.poll_review("fake-test-key", self.ledger, fetch=self.fetch)
        self.assertEqual((self.posts, self.gets), (1, 1))

    def test_invalid_response_identity_policy_or_usage_keeps_reserve(self):
        self.queue()
        invalid = [{"id": "resp_elsewhere"}, {"id": "../secret"}, {"model": "other-model"},
                   {"service_tier": "priority"}, {"status": "unexpected"}, {"usage": None},
                   {"usage": {"input_tokens": True, "output_tokens": 1}},
                   {"usage": {"input_tokens": 1, "output_tokens": -1}},
                   {"usage": {"input_tokens": 1, "output_tokens": module.MAX_OUTPUT + 1}},
                   {"usage": {"input_tokens": 100000000, "output_tokens": 1}}]
        for changes in invalid:
            with self.subTest(changes=changes):
                with self.assertRaisesRegex(module.ReviewBlocked, "poll_failed"):
                    module.poll_review("fake-test-key", self.ledger, fetch=lambda *args: self.response(**changes))
                row = self.state()["requests"][0]
                self.assertEqual(row["charged_micro_usd"], row["reserved_micro_usd"])

    def test_all_terminal_outcomes_account_usage_without_retry(self):
        for status in sorted(module.TERMINAL):
            with self.subTest(status=status):
                def send(payload, key):
                    self.send(payload, key)
                    return self.response(status)
                self.submit(status, send=send)
                self.assertEqual(self.state()["requests"][-1]["response_status"], status)
                with self.assertRaisesRegex(module.ReviewBlocked, "duplicate"):
                    self.submit(status)
                # Isolate fixtures while preserving the real three-attempt policy.
                self.ledger.unlink()

    def test_cache_write_cost_ceiling_and_reservation(self):
        def send(payload, key):
            self.send(payload, key)
            row = self.state()["requests"][0]
            minimum = ((len(payload["input"].encode()) + 2048) * 25 + 1) // 2 + module.MAX_OUTPUT * 50
            self.assertGreaterEqual(row["reserved_micro_usd"], minimum)
            return self.response(usage={"input_tokens": 2000, "output_tokens": 200,
                                       "input_tokens_details": {"cache_write_tokens": 2000}})
        result = self.submit(send=send)
        self.assertGreaterEqual(result["estimated_cost_usd_upper"], 0.035)
        self.assertLessEqual(result["estimated_cost_usd_upper"], 0.036)

    def test_budget_cap_applies_before_network(self):
        def expensive(payload, key):
            self.send(payload, key)
            return self.response(usage={"input_tokens": 100, "output_tokens": module.MAX_OUTPUT})
        self.submit(send=expensive)
        with self.assertRaisesRegex(module.ReviewBlocked, "reservation_exceeds"):
            self.submit("x" * 60000)
        self.assertEqual(self.posts, 1)

    def test_duplicate_label_prompt_and_attempt_cap(self):
        self.submit(label="first")
        with self.assertRaisesRegex(module.ReviewBlocked, "duplicate"):
            self.submit(max_output_tokens=100)
        with self.assertRaisesRegex(module.ReviewBlocked, "label_already"):
            self.submit("different", label="first")
        self.submit("second")
        self.submit("third")
        with self.assertRaisesRegex(module.ReviewBlocked, "request_limit"):
            self.submit("fourth")
        self.assertEqual(self.posts, 3)

    def test_corrupt_and_weakened_ledger_fail_closed(self):
        self.submit()
        original = self.state()
        mutations = [
            lambda s: s.update(schema=9),
            lambda s: s["policy"].update(budget_micro_usd=3000000),
            lambda s: s["requests"][0].update(charged_micro_usd=-1),
            lambda s: s["requests"][0].update(charged_micro_usd=0),
            lambda s: s["requests"][0].update(attempt="../../elsewhere"),
            lambda s: s["requests"][0].update(response_id="../../elsewhere"),
            lambda s: s["requests"][0].update(protocol="unknown"),
            lambda s: s["requests"][0].update(result_file="../elsewhere"),
            lambda s: s["requests"][0].update(status="unknown", charged_micro_usd=1),
        ]
        for change in mutations:
            state = copy.deepcopy(original)
            change(state)
            self.save(state)
            with self.assertRaises(module.ReviewBlocked):
                self.submit("different")
        self.ledger.write_text("{")
        with self.assertRaisesRegex(module.ReviewBlocked, "unreadable"):
            self.submit("different")
        self.assertEqual(self.posts, 1)

    def test_result_tampering_is_detected(self):
        self.submit()
        row = self.state()["requests"][0]
        module.atomic_write(self.ledger.parent / row["result_file"], self.response(output=[]))
        with self.assertRaisesRegex(module.ReviewBlocked, "integrity_mismatch"):
            module.review_result(self.ledger)

    def test_concurrent_caller_cannot_send(self):
        lock_path = self.ledger.with_suffix(".json.lock")
        lock_path.touch(mode=0o600)
        with lock_path.open("w") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            with self.assertRaisesRegex(module.ReviewBlocked, "already_running"):
                self.submit()
        self.assertEqual(self.posts, 0)

    def test_unsafe_files_and_permissions_block(self):
        self.save({"schema": 2, "policy": module.POLICY, "requests": []})
        self.ledger.chmod(0o644)
        with self.assertRaisesRegex(module.ReviewBlocked, "unsafe_private"):
            self.submit()
        self.ledger.chmod(0o600)
        target = self.ledger.with_name("alias.json")
        target.symlink_to(self.ledger)
        with self.assertRaisesRegex(module.ReviewBlocked, "nonsymlink"):
            module.submit_review("public code", "fake-test-key", target, send=self.send)
        self.assertEqual(self.posts, 0)

    def test_input_and_output_boundaries_prevent_network(self):
        cases = [("", {}), ("x" * 64001, {}), ("ع" * 32001, {}),
                 ("code", {"max_output_tokens": 0}), ("code", {"max_output_tokens": True}),
                 ("code", {"max_output_tokens": module.MAX_OUTPUT + 1}), ("code", {"label": "../label"})]
        for prompt, kwargs in cases:
            with self.subTest(kwargs=kwargs, length=len(prompt)):
                with self.assertRaises(module.ReviewBlocked):
                    self.submit(prompt, **kwargs)
        with self.assertRaisesRegex(module.ReviewBlocked, "api_key_missing"):
            module.submit_review("code", "", self.ledger, send=self.send)
        self.assertEqual(self.posts, 0)

    def test_poll_before_receipt_and_result_before_settlement_block(self):
        with self.assertRaisesRegex(module.ReviewBlocked, "no_resumable"):
            module.poll_review("fake-test-key", self.ledger, fetch=self.fetch)
        self.queue()
        with self.assertRaisesRegex(module.ReviewBlocked, "unavailable"):
            module.review_result(self.ledger)
        with self.assertRaisesRegex(module.ReviewBlocked, "no_resumable"):
            module.poll_review("fake-test-key", self.ledger, attempt="absent", fetch=self.fetch)
        self.assertEqual(self.gets, 0)


if __name__ == "__main__":
    unittest.main()
