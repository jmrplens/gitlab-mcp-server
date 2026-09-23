#!/usr/bin/env python3
"""Tests mcpb/linux/launch.sh, the Linux entry point of the Claude Desktop bundle.

The bundle's linux override runs `/bin/sh ${__dirname}/server/linux/launch.sh`,
and the launcher chooses between the two Linux release binaries by `uname -m`.
Claude Desktop reads the server's stdout as JSON-RPC and stops a server by
signalling the one PID it spawned, so what matters is narrow and exact: the
right binary runs, nothing but the server writes to stdout, a refusal is
explained on stderr, and the launcher's PID becomes the server's.

Each case lays out an extension directory the way Desktop extracts one, with a
space in its path, puts stub binaries beside the real launcher and a stub
`uname` on a curated PATH, and runs it from an unrelated working directory,
since Desktop gives a binary-type server no working directory of its own.

Run with:

    python3 -m unittest discover -s scripts -p 'mcpb_launch_sh_test.py'
"""

import os
import shutil
import signal
import stat
import subprocess
import tempfile
import time
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
LAUNCHER = os.path.join(ROOT, "mcpb", "linux", "launch.sh")

BINARIES = {
    "amd64": "gitlab-mcp-server-linux-amd64",
    "arm64": "gitlab-mcp-server-linux-arm64",
}

# A stand-in for a release binary. It reports who it is and what it was given
# on stderr, records its PID for the identity case, and touches stdout only in
# the mode that stands for a server answering on it.
STUB = """#!/bin/sh
printf 'stub %s\\n' "${0##*/}" >&2
for a in "$@"; do
  printf 'arg=[%s]\\n' "$a" >&2
done
if [ -n "${STUB_PID_FILE:-}" ]; then
  printf '%s\\n' "$$" > "$STUB_PID_FILE"
fi
case "${STUB_MODE:-}" in
  echo)
    IFS= read -r line
    printf '%s\\n' "$line"
    ;;
  wait)
    IFS= read -r line
    ;;
esac
exit 0
"""


def shells():
    """The POSIX shells available here, /bin/sh first since Desktop uses it."""
    found = [("sh", ["/bin/sh"])]
    if shutil.which("dash"):
        found.append(("dash", [shutil.which("dash")]))
    if shutil.which("busybox"):
        found.append(("busybox", [shutil.which("busybox"), "sh"]))
    if shutil.which("bash"):
        found.append(("bash-posix", [shutil.which("bash"), "--posix"]))
    return found


class McpbLaunchShTest(unittest.TestCase):
    """Drives the real launcher over stub binaries and a stub uname."""

    def setUp(self):
        self.work = tempfile.mkdtemp(prefix="mcpb-launch-")
        self.addCleanup(shutil.rmtree, self.work, True)
        # Desktop's own extension directory is
        # ~/.config/Claude/Claude Extensions/<id>, with a space in it.
        self.linux_dir = os.path.join(
            self.work, "Claude Extensions", "local.mcpb.fixture.gitlab-mcp-server", "server", "linux")
        os.makedirs(self.linux_dir)
        self.launcher = os.path.join(self.linux_dir, "launch.sh")
        shutil.copyfile(LAUNCHER, self.launcher)
        # Read permission only: /bin/sh runs it, so no execute bit is needed.
        os.chmod(self.launcher, 0o644)
        for name in BINARIES.values():
            self.write_stub(name, 0o755)

        self.bin_dir = os.path.join(self.work, "bin")
        os.makedirs(self.bin_dir)
        with open(os.path.join(self.bin_dir, "uname"), "w", encoding="utf-8") as fh:
            fh.write('#!/bin/sh\nprintf \'%s\\n\' "$STUB_MACHINE"\n')
        os.chmod(os.path.join(self.bin_dir, "uname"), 0o755)
        chmod = shutil.which("chmod")
        self.assertIsNotNone(chmod, "chmod must be on PATH to build the fixture")
        os.symlink(chmod, os.path.join(self.bin_dir, "chmod"))

        self.cwd = os.path.join(self.work, "elsewhere")
        os.makedirs(self.cwd)
        self.pid_file = os.path.join(self.work, "server.pid")

    def write_stub(self, name, mode):
        path = os.path.join(self.linux_dir, name)
        with open(path, "w", encoding="utf-8") as fh:
            fh.write(STUB)
        os.chmod(path, mode)
        return path

    def env(self, machine, **extra):
        env = {"PATH": self.bin_dir, "STUB_MACHINE": machine, "STUB_PID_FILE": self.pid_file}
        env.update(extra)
        return env

    def run_launcher(self, shell, machine, args=(), stdin=b"", cwd=None, launcher=None, **env):
        return subprocess.run(
            shell + [launcher or self.launcher, *args],
            input=stdin,
            capture_output=True,
            env=self.env(machine, **env),
            cwd=cwd or self.cwd,
            timeout=30,
            check=False,
        )

    def test_selects_the_binary_for_the_machine(self):
        cases = [
            ("x86_64", "amd64"),
            ("amd64", "amd64"),
            ("aarch64", "arm64"),
            ("arm64", "arm64"),
        ]
        for shell_name, shell in shells():
            for machine, arch in cases:
                with self.subTest(shell=shell_name, machine=machine):
                    result = self.run_launcher(shell, machine, args=["--version", "two words", ""])
                    stderr = result.stderr.decode()
                    self.assertEqual(result.returncode, 0, stderr)
                    self.assertEqual(result.stdout, b"", "the launcher wrote to stdout")
                    self.assertIn(f"stub {BINARIES[arch]}\n", stderr)
                    other = BINARIES["arm64" if arch == "amd64" else "amd64"]
                    self.assertNotIn(other, stderr)
                    self.assertIn("arg=[--version]\narg=[two words]\narg=[]\n", stderr)

    def test_refuses_an_unsupported_machine_on_stderr(self):
        for shell_name, shell in shells():
            for machine in ("riscv64", "armv7l", "i686", ""):
                with self.subTest(shell=shell_name, machine=machine):
                    result = self.run_launcher(shell, machine)
                    stderr = result.stderr.decode()
                    self.assertNotEqual(result.returncode, 0)
                    self.assertEqual(result.stdout, b"", "a refusal must not reach stdout")
                    self.assertIn(f'machine type "{machine}"', stderr)
                    self.assertNotIn("stub ", stderr, "no binary may run on a refusal")
                    self.assertFalse(os.path.exists(self.pid_file))

    def test_restores_a_missing_execute_bit(self):
        for shell_name, shell in shells():
            with self.subTest(shell=shell_name):
                path = self.write_stub(BINARIES["amd64"], 0o644)
                result = self.run_launcher(shell, "x86_64")
                self.assertEqual(result.returncode, 0, result.stderr.decode())
                self.assertEqual(result.stdout, b"")
                self.assertIn(f"stub {BINARIES['amd64']}\n", result.stderr.decode())
                self.assertTrue(os.stat(path).st_mode & stat.S_IXUSR)

    def test_reports_a_failed_chmod_on_stderr(self):
        with open(os.path.join(self.bin_dir, "chmod.fail"), "w", encoding="utf-8") as fh:
            fh.write("#!/bin/sh\necho 'chmod: fixture refusal' >&2\nexit 1\n")
        os.chmod(os.path.join(self.bin_dir, "chmod.fail"), 0o755)
        os.remove(os.path.join(self.bin_dir, "chmod"))
        os.rename(os.path.join(self.bin_dir, "chmod.fail"), os.path.join(self.bin_dir, "chmod"))
        self.write_stub(BINARIES["arm64"], 0o644)
        for shell_name, shell in shells():
            with self.subTest(shell=shell_name):
                result = self.run_launcher(shell, "aarch64")
                stderr = result.stderr.decode()
                self.assertEqual(result.returncode, 126, stderr)
                self.assertEqual(result.stdout, b"", "a failure must not reach stdout")
                self.assertIn("chmod: fixture refusal", stderr)
                self.assertIn("is not executable and chmod failed", stderr)
                self.assertNotIn("stub ", stderr)

    def test_reports_a_missing_binary_on_stderr(self):
        os.remove(os.path.join(self.linux_dir, BINARIES["amd64"]))
        for shell_name, shell in shells():
            with self.subTest(shell=shell_name):
                result = self.run_launcher(shell, "x86_64")
                stderr = result.stderr.decode()
                self.assertEqual(result.returncode, 127, stderr)
                self.assertEqual(result.stdout, b"", "a failure must not reach stdout")
                self.assertIn("gitlab-mcp-server-linux-amd64 is missing from the extension", stderr)

    def test_runs_from_its_own_directory_by_relative_name(self):
        for shell_name, shell in shells():
            with self.subTest(shell=shell_name):
                result = self.run_launcher(shell, "arm64", cwd=self.linux_dir, launcher="launch.sh")
                self.assertEqual(result.returncode, 0, result.stderr.decode())
                self.assertEqual(result.stdout, b"")
                self.assertIn(f"stub {BINARIES['arm64']}\n", result.stderr.decode())

    def test_hands_stdin_and_stdout_to_the_server(self):
        request = b'{"jsonrpc":"2.0","id":1,"method":"ping"}\n'
        for shell_name, shell in shells():
            with self.subTest(shell=shell_name):
                result = self.run_launcher(shell, "x86_64", stdin=request, STUB_MODE="echo")
                self.assertEqual(result.returncode, 0, result.stderr.decode())
                self.assertEqual(result.stdout, request)

    def test_the_server_takes_over_the_launcher_pid(self):
        for shell_name, shell in shells():
            with self.subTest(shell=shell_name):
                if os.path.exists(self.pid_file):
                    os.remove(self.pid_file)
                proc = subprocess.Popen(
                    shell + [self.launcher],
                    stdin=subprocess.PIPE,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    env=self.env("x86_64", STUB_MODE="wait"),
                    cwd=self.cwd,
                )
                try:
                    deadline = time.monotonic() + 10
                    while time.monotonic() < deadline and not self.pid_recorded():
                        time.sleep(0.02)
                    self.assertTrue(self.pid_recorded(), "the stub never started")
                    with open(self.pid_file, encoding="utf-8") as fh:
                        server_pid = int(fh.read().strip())
                    # exec, not a child: the PID Desktop spawned is the server.
                    self.assertEqual(server_pid, proc.pid)
                    # And the signal Desktop sends to that PID ends the server.
                    proc.send_signal(signal.SIGTERM)
                    stdout, _ = proc.communicate(timeout=10)
                    self.assertEqual(proc.returncode, -signal.SIGTERM)
                    self.assertEqual(stdout, b"")
                finally:
                    if proc.poll() is None:
                        proc.kill()
                        proc.communicate()

    def pid_recorded(self):
        try:
            with open(self.pid_file, encoding="utf-8") as fh:
                return fh.read().endswith("\n")
        except FileNotFoundError:
            return False

    def test_passes_shellcheck_as_posix_sh(self):
        shellcheck = shutil.which("shellcheck")
        if shellcheck is None:
            self.skipTest("shellcheck is not installed")
        result = subprocess.run(
            [shellcheck, "--shell=sh", LAUNCHER], capture_output=True, timeout=60, check=False)
        self.assertEqual(result.returncode, 0, result.stdout.decode() + result.stderr.decode())


if __name__ == "__main__":
    unittest.main()
