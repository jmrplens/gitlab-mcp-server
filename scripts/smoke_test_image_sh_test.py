#!/usr/bin/env python3
"""Tests scripts/smoke-test-image.sh against a stand-in docker.

The smoke test starts the image on each platform it claims and checks three
things: the binary starts and reports the release's version, the image
carries this repository's LICENSE, and it carries the THIRD_PARTY_NOTICES
the image's builder generated for its own binary, which name the platform
the image was built for. Each refusal is exercised here with a stand-in for
docker first on PATH: `--version` prints the version FAKE_VERSION names, and
`--entrypoint /bin/cat` prints the file FAKE_LICENSE or FAKE_NOTICES names,
or fails the way cat does when the variable is empty.

Run with:

    python3 -m unittest discover -s scripts -p 'smoke_test_image_sh_test.py'
"""

import os
import shutil
import subprocess
import tempfile
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
SCRIPT = os.path.join(ROOT, "scripts", "smoke-test-image.sh")
VERSION = "9.8.7"
IMAGE = "gitlab-mcp-server:smoke-amd64"
PLATFORM = "linux/amd64"

DOCKER = """#!/usr/bin/env bash
entrypoint="" last=""
for arg; do
  last="$arg"
done
while [ $# -gt 0 ]; do
  case "$1" in
    --entrypoint) entrypoint="$2"; shift 2 ;;
    *) shift ;;
  esac
done
if [ -z "$entrypoint" ]; then
  [ "$last" = "--version" ] || { echo "unexpected docker call" >&2; exit 2; }
  echo "gitlab-mcp-server $FAKE_VERSION (commit 0123456)"
  exit 0
fi
case "$last" in
  */LICENSE) file="$FAKE_LICENSE" ;;
  */THIRD_PARTY_NOTICES) file="$FAKE_NOTICES" ;;
  *) echo "unexpected path $last" >&2; exit 2 ;;
esac
if [ -z "$file" ]; then
  echo "cat: can't open '$last': No such file or directory" >&2
  exit 1
fi
cat "$file"
"""


def notices(platform):
    """What cmd/gen_third_party_notices writes for one build: its header,
    and a body as long as the real one's (some 400 KB), which is what made a
    piped check die of SIGPIPE against the first image built with them."""
    return (
        "Third-party notices for gitlab-mcp-server\n\n"
        "Module:    github.com/jmrplens/gitlab-mcp-server/v3 (devel)\n"
        "Toolchain: go1.27.1\n"
        "Builds:    " + platform + "\n\n" + "licence text of a linked module\n" * 13000
    )


class SmokeTestImageTest(unittest.TestCase):
    """Runs the real script with a stand-in docker."""

    def setUp(self):
        if shutil.which("bash") is None:
            # GitHub sets CI on every step; there a missing tool is a failure.
            if os.environ.get("CI"):
                self.fail("the script needs bash, and CI must run these cases rather than skip them")
            self.skipTest("the script needs bash")
        self.work = tempfile.mkdtemp(prefix="smoke-test-image-")
        self.addCleanup(shutil.rmtree, self.work, True)
        self.bin = os.path.join(self.work, "bin")
        os.makedirs(self.bin)
        docker = os.path.join(self.bin, "docker")
        with open(docker, "w", encoding="utf-8") as fh:
            fh.write(DOCKER)
        os.chmod(docker, 0o755)
        self.licence = os.path.join(ROOT, "LICENSE")
        self.notices = self.write("notices", notices(PLATFORM))

    def write(self, name, text):
        path = os.path.join(self.work, name)
        with open(path, "w", encoding="utf-8") as fh:
            fh.write(text)
        return path

    def smoke(self, version=VERSION, licence=None, notices_file=None):
        env = dict(os.environ)
        env.update(
            PATH=self.bin + os.pathsep + env.get("PATH", ""),
            FAKE_VERSION=version,
            FAKE_LICENSE=self.licence if licence is None else licence,
            FAKE_NOTICES=self.notices if notices_file is None else notices_file,
        )
        return subprocess.run(
            ["bash", SCRIPT, VERSION, IMAGE + "=" + PLATFORM],
            env=env, capture_output=True, text=True, timeout=60, check=False,
        )

    def test_an_image_with_its_version_licence_and_notices_passes(self):
        result = self.smoke()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("licence and third-party notices for linux/amd64 present", result.stdout)
        self.assertIn("carry their licence and notices", result.stdout)

    def test_refuses_each_departure(self):
        cases = [
            ("another version", dict(version="1.0.0"), "started but printed"),
            ("no licence", dict(licence=""), "carries no /usr/share/licenses/gitlab-mcp-server/LICENSE"),
            ("another licence", dict(licence=self.write("other-licence", "Not the MIT License\n")),
             "LICENSE that is not this repository's"),
            ("no notices", dict(notices_file=""),
             "carries no /usr/share/licenses/gitlab-mcp-server/THIRD_PARTY_NOTICES"),
            ("notices without the generator's header",
             dict(notices_file=self.write("headless", "Builds:    linux/amd64\n")),
             "THIRD_PARTY_NOTICES that are not the generator's for linux/amd64"),
            ("notices of another build",
             dict(notices_file=self.write("arm64", notices("linux/arm64"))),
             "THIRD_PARTY_NOTICES that are not the generator's for linux/amd64"),
            ("notices of a build covering more than the image",
             dict(notices_file=self.write("release", notices("linux/amd64, linux/arm64"))),
             "THIRD_PARTY_NOTICES that are not the generator's for linux/amd64"),
        ]
        for name, change, want in cases:
            with self.subTest(name):
                result = self.smoke(**change)
                self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
                self.assertIn(want, result.stderr)
                self.assertIn("1 platform(s) failed the smoke test", result.stderr)


if __name__ == "__main__":
    unittest.main()
