#!/usr/bin/env python3
"""Verify the configured panel and referenced assets through the actual HTTP server."""
import argparse
import json
import pathlib
import runpy
import subprocess
import urllib.error
import urllib.parse
import urllib.request
from html.parser import HTMLParser


class Assets(HTMLParser):
    def __init__(self):
        super().__init__()
        self.urls = set()

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if tag == "script" and attrs.get("src"):
            self.urls.add(attrs["src"])
        if tag == "link" and "stylesheet" in attrs.get("rel", "").split():
            self.urls.add(attrs["href"])


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def verify(base, panel_path, static_root):
    root = pathlib.Path(static_root).resolve()
    path = urllib.parse.urlsplit(panel_path)
    if (not panel_path.startswith("/") or panel_path.startswith("//")
            or path.scheme or path.netloc or path.query or path.fragment):
        raise ValueError("Invalid configured panel path")
    expected = (root / "index.html").read_bytes()
    assets = Assets()
    assets.feed(expected.decode("utf-8"))
    if not assets.urls:
        raise ValueError("Panel references no assets")
    requests = [(panel_path, expected)]
    for url in sorted(assets.urls):
        parsed = urllib.parse.urlsplit(urllib.parse.urljoin(base + panel_path, url))
        decoded = urllib.parse.unquote(parsed.path)
        if (parsed.scheme != "http" or parsed.netloc != urllib.parse.urlsplit(base).netloc or parsed.fragment
                or not decoded.startswith("/static/")
                or ".." in pathlib.PurePosixPath(decoded).parts):
            raise ValueError("Invalid panel asset path")
        local = (root / decoded[len("/static/"):]).resolve()
        if root not in local.parents or not local.is_file():
            raise ValueError("Missing panel asset")
        requests.append((urllib.parse.urlunsplit(("", "", parsed.path, parsed.query, "")), local.read_bytes()))
    # Local probes must not leave the host via proxy environment variables.
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    for url, content in requests:
        try:
            with opener.open(base + url, timeout=5) as response:
                if response.status != 200 or response.read(len(content) + 1) != content:
                    raise RuntimeError("Panel or asset differs from the release")
        except urllib.error.HTTPError as error:
            error.close()
            raise RuntimeError("Panel or asset HTTP check failed") from None
    return {"panel_http": 200, "assets_verified": len(assets.urls)}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--bootstrap", required=True)
    parser.add_argument("--etc", required=True)
    parser.add_argument("--static-root", required=True)
    args = parser.parse_args()
    helpers = runpy.run_path(args.bootstrap)
    values = helpers["read_environment"](pathlib.Path(args.etc) / "env")
    sql = "SELECT json_build_object('port',listen_port,'path',web_path) FROM panel_settings WHERE id=true"
    settings = json.loads(subprocess.check_output(
        ["psql", "-XAt", "-v", "ON_ERROR_STOP=1", "-c", sql],
        env=helpers["database_environment"](values), text=True, timeout=15))
    port = int(settings["port"])
    if not 1024 <= port <= 65535:
        raise ValueError("Invalid configured panel port")
    print(json.dumps(verify("http://127.0.0.1:" + str(port), settings["path"], args.static_root)))


if __name__ == "__main__":
    main()
