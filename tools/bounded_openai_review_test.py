import importlib.util
import fcntl
import json
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("bounded_review", Path(__file__).with_name("bounded-openai-review.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

class BoundedReviewTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.ledger = Path(self.directory.name) / "ledger.json"
        self.calls = 0

    def reply(self, payload, key):
        self.calls += 1
        self.assertEqual(payload["model"], "gpt-6-astra")
        self.assertEqual(payload["reasoning"], {"effort": "max"})
        self.assertFalse(payload["store"])
        self.assertEqual(payload["service_tier"], "default")
        self.assertEqual(payload["prompt_cache_options"], {"ttl": "30m"})
        self.assertEqual(key, "fixture-key")
        return {"id": "fixture-response", "model": module.MODEL, "status": "completed",
                "usage": {"input_tokens": 100, "output_tokens": 200},
                "output": [{"content": [{"type": "output_text", "text": "review fixture"}]}]}

    def call(self, prompt="code fixture", **kw):
        return module.review(prompt, "fixture-key", self.ledger, send=kw.get("send", self.reply))

    def test_missing_key_or_oversized_input_never_sends(self):
        with self.assertRaisesRegex(module.ReviewBlocked, "api_key_missing"):
            module.review("code", None, self.ledger, send=self.reply)
        with self.assertRaisesRegex(module.ReviewBlocked, "too_large"):
            self.call("x" * (module.MAX_INPUT_BYTES + 1))
        self.assertEqual(self.calls, 0)
        self.assertFalse(self.ledger.exists())

    def test_usage_is_recorded_without_prompt_or_key(self):
        result = self.call()
        self.assertEqual(result["estimated_cost_usd_upper"], 0.0113)
        text = self.ledger.read_text()
        self.assertNotIn("code fixture", text)
        self.assertNotIn("fixture-key", text)
        self.assertEqual(self.ledger.stat().st_mode & 0o777, 0o600)
        self.assertEqual(json.loads(text)["requests"][0]["status"], "completed")

    def test_cache_write_pricing_is_covered(self):
        def cached_write(payload, key):
            row = json.loads(self.ledger.read_text())['requests'][0]
            minimum = ((len(payload['input'].encode()) + 2048) * 25 + 1) // 2 + module.MAX_OUTPUT * 50
            self.assertGreaterEqual(row['reserved_micro_usd'], minimum)
            result = self.reply(payload, key)
            result['usage'] = {'input_tokens': 2000, 'output_tokens': 200,
                               'input_tokens_details': {'cache_write_tokens': 2000}}
            return result
        result = self.call('review cached source', send=cached_write)
        # Published 30-minute write rate is $12.50/M, output is $50/M.
        # Cache-write tokens replace ordinary input, rather than adding to it.
        self.assertGreaterEqual(result['estimated_cost_usd_upper'], 0.035)
        self.assertLessEqual(result['estimated_cost_usd_upper'], 0.036)

    def test_duplicate_and_request_limit_block_before_network(self):
        self.call()
        with self.assertRaisesRegex(module.ReviewBlocked, "duplicate"):
            self.call()
        self.call("second")
        self.call("third")
        with self.assertRaisesRegex(module.ReviewBlocked, "request_limit"):
            self.call("fourth")
        self.assertEqual(self.calls, 3)

    def test_unknown_outcome_keeps_reservation_and_blocks_further_calls(self):
        def timeout(payload, key):
            raise TimeoutError()
        with self.assertRaisesRegex(module.ReviewBlocked, "uncertain"):
            self.call(send=timeout)
        row = json.loads(self.ledger.read_text())["requests"][0]
        self.assertEqual(row["charged_micro_usd"], row["reserved_micro_usd"])
        with self.assertRaisesRegex(module.ReviewBlocked, "unresolved_request"):
            self.call("new prompt")
        self.assertEqual(self.calls, 0)

    def test_budget_reserves_maximum_before_network(self):
        def expensive(payload, key):
            result = self.reply(payload, key)
            result["usage"]["output_tokens"] = module.MAX_OUTPUT
            return result
        self.call(send=expensive)
        with self.assertRaisesRegex(module.ReviewBlocked, "reservation_exceeds"):
            self.call("x" * 60000)
        self.assertEqual(self.calls, 1)

    def test_corrupt_state_and_changed_policy_fail_closed(self):
        self.ledger.write_text("{")
        with self.assertRaisesRegex(module.ReviewBlocked, "unreadable"):
            self.call()
        self.ledger.unlink()
        self.call()
        state = json.loads(self.ledger.read_text())
        state["requests"][0]["charged_micro_usd"] = -1
        self.ledger.write_text(json.dumps(state))
        with self.assertRaisesRegex(module.ReviewBlocked, "invalid"):
            self.call("different")
        state["policy"]["budget_micro_usd"] += 1
        self.ledger.write_text(json.dumps(state))
        with self.assertRaisesRegex(module.ReviewBlocked, "policy_mismatch"):
            self.call("different")
        self.assertEqual(self.calls, 1)

    def test_concurrent_caller_cannot_send(self):
        with self.ledger.with_suffix(".json.lock").open("w") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            with self.assertRaisesRegex(module.ReviewBlocked, "already_running"):
                self.call()
        self.assertEqual(self.calls, 0)

    def test_incomplete_response_records_usage_and_does_not_retry(self):
        def incomplete(payload, key):
            result = self.reply(payload, key)
            result["status"] = "incomplete"
            result["output"] = []
            return result
        with self.assertRaisesRegex(module.ReviewBlocked, "response_incomplete"):
            self.call(send=incomplete)
        self.assertEqual(json.loads(self.ledger.read_text())["requests"][0]["status"], "completed")
        with self.assertRaisesRegex(module.ReviewBlocked, "duplicate"):
            self.call()
        self.assertEqual(self.calls, 1)

if __name__ == "__main__":
    unittest.main()
