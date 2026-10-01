#!/usr/bin/env python3
"""Tests how scripts/update-server-json-sha.sh declares the Claude Desktop bundles in server.json.

The release stamps server.json with the bundles it built. A registry entry has
no platform field, so the stamp declares one bundle per operating system, each
with its own identifier and fileSha256, in place of whatever mcpb entries
server.json held: the first release to run it turns the single universal entry
into three, and a re-run with the same bundles writes the same file. What it
must refuse is a set a client could not choose from: a bundle serving several
systems (the universal one), a system no bundle serves, a system two serve, a
bundle named twice, and a file that is not a bundle. A refused set leaves
server.json as it was, because the bundles are read before anything is written.

Each case runs the real script in a scratch directory holding a copy of the
repository's server.json and bundles built here: zips carrying only the
manifest the script reads.

Run with:

    python3 -m unittest discover -s scripts -p 'update_server_json_sha_sh_test.py'
"""

import hashlib
import json
import os
import shutil
import subprocess
import tempfile
import unittest
import zipfile

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
SCRIPT = os.path.join(ROOT, "scripts", "update-server-json-sha.sh")
VERSION = "9.8.7"
DIGEST = "sha256:" + "a" * 64
RELEASE = "https://github.com/jmrplens/gitlab-mcp-server/releases/download/v" + VERSION + "/"
PER_OS = {
    "gitlab-mcp-server-darwin.mcpb": ["darwin"],
    "gitlab-mcp-server-windows.mcpb": ["win32"],
    "gitlab-mcp-server-linux.mcpb": ["linux"],
}


class UpdateServerJsonMcpbTest(unittest.TestCase):
    """Runs the real stamp over bundles built in a scratch directory."""

    def setUp(self):
        missing = [tool for tool in ("bash", "jq", "unzip", "sha256sum") if shutil.which(tool) is None]
        if missing:
            reason = "the stamp needs " + ", ".join(missing)
            # A skip keeps the job green, so on a runner without these tools
            # every case here would stop running and nobody would be told.
            # GitHub sets CI on every step; there a missing tool is a failure.
            if os.environ.get("CI"):
                self.fail(reason + ", and CI must run these cases rather than skip them")
            self.skipTest(reason)
        self.work = tempfile.mkdtemp(prefix="update-server-json-")
        self.addCleanup(shutil.rmtree, self.work, True)
        shutil.copyfile(os.path.join(ROOT, "server.json"), self.path("server.json"))
        with open(self.path("server.json"), encoding="utf-8") as fh:
            self.original = json.load(fh)
        # A binary nothing declares: the stamp's own floor is that something
        # got a hash, and the bundles are what get one here.
        with open(self.path("checksums.txt"), "w", encoding="utf-8") as fh:
            fh.write("0" * 64 + "  gitlab-mcp-server-linux-amd64\n")

    def path(self, *parts):
        return os.path.join(self.work, *parts)

    def bundle(self, name, platforms, manifest=True):
        """Writes a bundle whose manifest lists platforms, and returns its path."""
        path = self.path("dist", name)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with zipfile.ZipFile(path, "w") as archive:
            if manifest:
                archive.writestr("manifest.json", json.dumps({"compatibility": {"platforms": platforms}}))
            archive.writestr("server/stand-in", name)
        return path

    def per_os_bundles(self):
        return [self.bundle(name, platforms) for name, platforms in PER_OS.items()]

    def stamp(self, bundles):
        return subprocess.run(
            ["bash", SCRIPT, "checksums.txt", VERSION, ",".join(bundles), DIGEST],
            cwd=self.work,
            capture_output=True,
            timeout=60,
            check=False,
        )

    def stamped(self):
        with open(self.path("server.json"), encoding="utf-8") as fh:
            return json.load(fh)

    def assert_refused(self, result, *messages):
        stderr = result.stderr.decode()
        self.assertEqual(result.returncode, 1, stderr)
        for message in messages:
            self.assertIn(message, stderr)
        self.assertEqual(self.stamped(), self.original, "a refused set changed server.json")

    def test_declares_one_entry_per_bundle_in_place_of_the_universal_one(self):
        bundles = self.per_os_bundles()
        result = self.stamp(bundles)
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        packages = self.stamped()["packages"]
        template = next(p for p in self.original["packages"] if p["registryType"] == "mcpb")
        mcpb = [p for p in packages if p["registryType"] == "mcpb"]
        self.assertEqual(len(mcpb), 3)
        for entry, path in zip(mcpb, bundles):
            name = os.path.basename(path)
            with self.subTest(bundle=name):
                with open(path, "rb") as fh:
                    digest = hashlib.sha256(fh.read()).hexdigest()
                self.assertEqual(entry["identifier"], RELEASE + name)
                self.assertEqual(entry["fileSha256"], digest)
                self.assertEqual(entry["version"], VERSION)
                # Everything else is the entry the bundles replaced, field for
                # field and in the same order.
                self.assertEqual(list(entry), list(template))
                for key in template:
                    if key not in ("identifier", "fileSha256", "version"):
                        self.assertEqual(entry[key], template[key], key)
        # The bundles take the universal entry's place, ahead of every other
        # package, and the other packages keep their order.
        self.assertEqual([p["registryType"] for p in packages[:3]], ["mcpb"] * 3)
        self.assertEqual(
            [p["registryType"] for p in packages[3:]],
            [p["registryType"] for p in self.original["packages"] if p["registryType"] != "mcpb"],
        )
        for name in PER_OS:
            self.assertIn("SHA256 for " + name, result.stdout.decode())

    def test_a_second_stamp_with_the_same_bundles_writes_the_same_file(self):
        bundles = self.per_os_bundles()
        first = self.stamp(bundles)
        self.assertEqual(first.returncode, 0, first.stderr.decode())
        with open(self.path("server.json"), "rb") as fh:
            once = fh.read()
        second = self.stamp(bundles)
        self.assertEqual(second.returncode, 0, second.stderr.decode())
        with open(self.path("server.json"), "rb") as fh:
            self.assertEqual(fh.read(), once)

    def test_refuses_the_universal_bundle(self):
        universal = self.bundle("gitlab-mcp-server.mcpb", ["darwin", "win32", "linux"])
        cases = [
            ("alone", [universal], [
                "gitlab-mcp-server.mcpb serves 3 platforms (darwin, win32, linux), and a declared bundle serves exactly one",
            ]),
            ("beside the per-OS bundles", self.per_os_bundles() + [universal], [
                "gitlab-mcp-server.mcpb serves 3 platforms",
                "darwin is served by 2 bundles",
                "win32 is served by 2 bundles",
                "linux is served by 2 bundles",
            ]),
        ]
        for name, bundles, messages in cases:
            with self.subTest(case=name):
                self.assert_refused(self.stamp(bundles), *messages)

    def test_refuses_a_set_that_is_not_one_bundle_per_system(self):
        darwin, windows, linux = self.per_os_bundles()
        second_linux = self.bundle("gitlab-mcp-server-linux-musl.mcpb", ["linux"])
        freebsd = self.bundle("gitlab-mcp-server-freebsd.mcpb", ["freebsd"])
        empty = self.bundle("gitlab-mcp-server-none.mcpb", [])
        cases = [
            ("a system no bundle serves", [darwin, windows], ["no bundle given serves linux"]),
            ("a system two bundles serve", [darwin, windows, linux, second_linux], ["linux is served by 2 bundles"]),
            ("a bundle given twice", [darwin, windows, linux, linux], [
                "gitlab-mcp-server-linux.mcpb is given more than once",
                "linux is served by 2 bundles",
            ]),
            ("a platform Claude Desktop does not report", [darwin, windows, linux, freebsd], [
                "freebsd is not a platform Claude Desktop reports",
            ]),
            ("a bundle that serves nothing", [darwin, windows, linux, empty], [
                "gitlab-mcp-server-none.mcpb serves 0 platforms",
            ]),
        ]
        for name, bundles, messages in cases:
            with self.subTest(case=name):
                self.assert_refused(
                    self.stamp(bundles), *messages,
                    "the bundles given are not one per operating system; server.json was left as it was")

    def test_refuses_a_file_that_is_not_a_bundle(self):
        darwin, windows, _ = self.per_os_bundles()
        no_manifest = self.bundle("gitlab-mcp-server-linux.mcpb", ["linux"], manifest=False)
        not_a_zip = self.path("dist", "gitlab-mcp-server-plain.mcpb")
        with open(not_a_zip, "wb") as fh:
            fh.write(b"\x7fELF not a zip")
        listed_as_text = self.path("dist", "gitlab-mcp-server-text.mcpb")
        with zipfile.ZipFile(listed_as_text, "w") as archive:
            archive.writestr("manifest.json", json.dumps({"compatibility": {"platforms": "linux"}}))
        cases = [
            ("no manifest", no_manifest, "gitlab-mcp-server-linux.mcpb carries no manifest.json"),
            ("not a zip", not_a_zip, "gitlab-mcp-server-plain.mcpb carries no manifest.json"),
            ("platforms that are not a list", listed_as_text, "gitlab-mcp-server-text.mcpb carries no manifest.json"),
            ("a missing file", self.path("dist", "absent.mcpb"), "mcpb bundle not found: "),
        ]
        for name, bad, message in cases:
            with self.subTest(case=name):
                self.assert_refused(self.stamp([darwin, windows, bad]), message)

    def test_refuses_a_server_json_with_no_mcpb_entry_to_copy(self):
        stripped = dict(self.original)
        stripped["packages"] = [p for p in self.original["packages"] if p["registryType"] != "mcpb"]
        with open(self.path("server.json"), "w", encoding="utf-8") as fh:
            json.dump(stripped, fh, indent=2)
        self.original = stripped
        self.assert_refused(
            self.stamp(self.per_os_bundles()),
            "server.json declares no mcpb entry to take the bundles' transport, environment and release URL from",
        )


if __name__ == "__main__":
    unittest.main()
