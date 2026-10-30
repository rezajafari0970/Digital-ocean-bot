#!/usr/bin/env python3
"""Root integration regression: restrictive clone modes and real HTTP panel checks."""
import http.server
import os
import pathlib
import pwd
import runpy
import subprocess
import tempfile
import threading
import unittest

HERE = pathlib.Path(__file__).resolve().parent
VERIFY = runpy.run_path(str(HERE / "verify-panel.py"))["verify"]


class Permissions(unittest.TestCase):
    def setUp(self):
        if os.geteuid() != 0:
            self.skipTest("Run as root with digitaloceanbot present")
        self.user = pwd.getpwnam("digitaloceanbot")
        self.tmp = tempfile.TemporaryDirectory(prefix="dob-permissions-")
        self.root = pathlib.Path(self.tmp.name)
        self.root.chmod(0o755)
        self.stage = self.root / "stage"
        for name in ("bin", "migrations", "web/static/nested"):
            (self.stage / name).mkdir(parents=True, mode=0o700, exist_ok=True)
        for name in ("bin/test", "bin/provider-browser-runner", "migrations/1.sql", "web/static/index.html", "web/static/nested/test.js"):
            p = self.stage / name
            p.write_text("fixture")
            p.chmod(0o600)
        for p in self.stage.rglob("*"):
            if p.is_dir():
                p.chmod(0o700)
        self.stage.chmod(0o700)

    def tearDown(self):
        if hasattr(self, "tmp"):
            self.tmp.cleanup()

    def prepare(self):
        # An if-call suppresses Bash errexit; the helper must return failure itself.
        code = 'source "$1"; DOB_STAGE="$2"; if dob_prepare_stage_permissions; then exit 0; else exit 19; fi'
        return subprocess.run(["bash", "-c", code, "test", str(HERE / "service-topology.sh"), str(self.stage)],
                              capture_output=True, text=True)

    def test_restrictive_clone_readable_and_never_writable(self):
        denied = subprocess.run(["runuser", "-u", "digitaloceanbot", "--", "cat",
                                 str(self.stage / "web/static/index.html")], capture_output=True)
        self.assertNotEqual(denied.returncode, 0)
        result = self.prepare()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.stage / "bin/provider-browser-runner").stat().st_mode & 0o777, 0o755)
        self.assertEqual((self.stage / "bin/test").stat().st_mode & 0o777, 0o750)
        for p in [self.stage / "web", *list((self.stage / "web").rglob("*"))]:
            self.assertEqual(p.stat().st_mode & 0o777, 0o750 if p.is_dir() else 0o640)
            self.assertEqual(p.stat().st_uid, 0)
            self.assertEqual(p.stat().st_gid, self.user.pw_gid)
            code = "import os,sys; p=sys.argv[1]; assert os.access(p,os.R_OK); assert not os.access(p,os.W_OK); assert os.access(p,os.X_OK) if os.path.isdir(p) else bool(open(p,'rb').read())"
            subprocess.run(["runuser", "-u", "digitaloceanbot", "--", "python3", "-c", code, str(p)], check=True)

    def test_missing_index_rejected(self):
        (self.stage / "web/static/index.html").unlink()
        self.assertEqual(self.prepare().returncode, 19)

    def test_symlink_and_dangling_link_rejected_before_mutation(self):
        outside = self.root / "outside"
        outside.write_text("private fixture")
        outside.chmod(0o600)
        link = self.stage / "web/static/link"
        for target in (outside, self.root / "absent"):
            link.symlink_to(target)
            self.assertEqual(self.prepare().returncode, 19)
            self.assertEqual(outside.stat().st_mode & 0o777, 0o600)
            self.assertEqual((self.stage / "web/static").stat().st_mode & 0o777, 0o700)
            link.unlink()

    def test_special_file_rejected(self):
        os.mkfifo(self.stage / "web/static/pipe")
        self.assertEqual(self.prepare().returncode, 19)

    def test_inaccessible_ancestor_rejected(self):
        self.root.chmod(0o700)
        self.assertEqual(self.prepare().returncode, 19)

    def test_permissive_source_cannot_grant_writes(self):
        for p in [self.stage, *self.stage.rglob("*")]:
            p.chmod(0o777)
        self.assertEqual(self.prepare().returncode, 0)
        self.assertEqual((self.stage / "web/static/index.html").stat().st_mode & 0o777, 0o640)


class HTTPGate(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="dob-http-gate-")
        self.root = pathlib.Path(self.tmp.name)
        (self.root / "nested").mkdir()
        (self.root / "index.html").write_text('<!doctype html><link rel="stylesheet" href="/static/test.css?v=1"><script src="../static/nested/app.js?v=2"></script>')
        (self.root / "test.css").write_text("body{}")
        (self.root / "nested/app.js").write_text("window.fixture=true;")
        self.responses = {
            "/custom/panel": (200, (self.root / "index.html").read_bytes()),
            "/static/test.css?v=1": (200, b"body{}"),
            "/static/nested/app.js?v=2": (200, b"window.fixture=true;"),
        }
        responses = self.responses

        class Handler(http.server.BaseHTTPRequestHandler):
            def do_GET(self):
                status, content = responses.get(self.path, (404, b"missing"))
                self.send_response(status)
                if status == 302:
                    self.send_header("Location", "/custom/panel")
                self.end_headers()
                self.wfile.write(content)

            def log_message(self, *args):
                pass

        self.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.base = "http://127.0.0.1:" + str(self.server.server_port)

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()
        self.tmp.cleanup()

    def test_custom_path_nested_query_and_relative_assets(self):
        self.assertEqual(VERIFY(self.base, "/custom/panel", self.root), {"panel_http": 200, "assets_verified": 2})

    def test_forbidden_html_rejected(self):
        self.responses["/custom/panel"] = (403, b"403 Forbidden")
        with self.assertRaises(Exception):
            VERIFY(self.base, "/custom/panel", self.root)

    def test_asset_failure_and_200_substitution_rejected(self):
        key = "/static/nested/app.js?v=2"
        for response in ((403, b"denied"), (404, b"missing"), (200, b"<!doctype html>")):
            self.responses[key] = response
            with self.assertRaises(Exception):
                VERIFY(self.base, "/custom/panel", self.root)

    def test_redirect_rejected(self):
        self.responses["/static/test.css?v=1"] = (302, b"")
        with self.assertRaises(Exception):
            VERIFY(self.base, "/custom/panel", self.root)

    def test_external_reference_rejected_without_request(self):
        (self.root / "index.html").write_text('<script src="https://invalid.example/app.js"></script>')
        with self.assertRaises(ValueError):
            VERIFY(self.base, "/custom/panel", self.root)

    def test_proxy_environment_ignored(self):
        old = {k: os.environ.get(k) for k in ("http_proxy", "HTTP_PROXY", "no_proxy", "NO_PROXY")}
        os.environ.update(http_proxy="http://127.0.0.1:1", HTTP_PROXY="http://127.0.0.1:1", no_proxy="", NO_PROXY="")
        try:
            self.assertEqual(VERIFY(self.base, "/custom/panel", self.root)["assets_verified"], 2)
        finally:
            for k, v in old.items():
                if v is None:
                    os.environ.pop(k, None)
                else:
                    os.environ[k] = v


if __name__ == "__main__":
    unittest.main(verbosity=2)
