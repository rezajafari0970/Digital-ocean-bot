import json
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from owned_cluster import ensure_cluster

class OwnedClusterTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.config = self.root / "config"
        self.data = self.root / "data"
        self.state = {"status": "pending", "cluster": {"version": 16, "name": "dob" + "a"*16, "port": 55432}}
        self.calls = []
        self.saved = []
        self.actual = {"system_identifier": "123456789", "checksums": "on", "listen_addresses": "127.0.0.1,::1", "port": "55432",
                       "version": 16, "data_directory": str(self.data / "16" / self.state["cluster"]["name"]),
                       "foreign_databases": 0, "foreign_roles": 0}
        self.confdir = self.config / "16" / self.state["cluster"]["name"]

    def command(self, args, **kw):
        self.calls.append(args)
        if args[0] == "pg_createcluster":
            self.confdir.mkdir(parents=True)
        return SimpleNamespace(returncode=0, stdout=json.dumps(self.actual))

    def ensure(self):
        return ensure_cluster(self.state, lambda s: self.saved.append(json.loads(json.dumps(s))),
                              run=self.command, config_root=self.config, data_root=self.data)

    def test_fresh_cluster_explicit_checksums_local_binding_and_peer_target(self):
        self.assertEqual(self.ensure(), 55432)
        create = self.calls[0]
        self.assertIn("--data-checksums", create)
        self.assertIn("--auth-local=peer", create)
        self.assertIn("--auth-host=scram-sha-256", create)
        self.assertIn("listen_addresses=127.0.0.1,::1", create)
        self.assertIn("55432", self.calls[-1])
        self.assertEqual(self.saved[0]["cluster"]["system_identifier"], "123456789")

    def test_resume_after_create_does_not_create_again(self):
        self.confdir.mkdir(parents=True)
        self.ensure()
        self.assertFalse(any(c[0] == "pg_createcluster" for c in self.calls))
        self.ensure()
        self.assertEqual(len(self.saved), 1)

    def test_existing_identifier_must_match(self):
        self.confdir.mkdir(parents=True)
        self.state["cluster"]["system_identifier"] = "different"
        with self.assertRaisesRegex(RuntimeError, "replaced"):
            self.ensure()
        self.assertEqual(self.saved, [])

    def test_unverified_foreign_database_never_adopted(self):
        self.confdir.mkdir(parents=True)
        self.actual["foreign_databases"] = 1
        with self.assertRaisesRegex(RuntimeError, "existing user data"):
            self.ensure()
        self.assertEqual(self.saved, [])

    def test_disabled_checksums_wrong_directory_and_wrong_port_are_rejected(self):
        self.confdir.mkdir(parents=True)
        for field, value in (("listen_addresses", "*"), ("checksums", "off"), ("port", "5432"), ("data_directory", "/unowned")):
            original = self.actual[field]
            self.actual[field] = value
            with self.subTest(field=field), self.assertRaisesRegex(RuntimeError, "mismatch"):
                self.ensure()
            self.actual[field] = original

    def test_partial_data_directory_is_preserved(self):
        Path(self.actual["data_directory"]).mkdir(parents=True)
        with self.assertRaisesRegex(RuntimeError, "preserved"):
            self.ensure()
        self.assertEqual(self.calls, [])

    def test_invalid_intent_cannot_run_a_command(self):
        self.state["cluster"]["name"] = "../main"
        with self.assertRaisesRegex(RuntimeError, "Invalid"):
            self.ensure()
        self.assertEqual(self.calls, [])
