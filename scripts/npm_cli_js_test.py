#!/usr/bin/env python3
"""Tests npm/gitlab-mcp-server/cli.js, the launcher the npm package runs.

The launcher resolves the platform package's binary and runs it with stdio
inherited, so a client talking MCP to the launcher is talking to the server.
What it owes the client beyond that is narrow and exact: argv reaches the
server unchanged, the server's exit code or terminating signal comes back as
the launcher's own, a signal sent to the launcher reaches the server, and under
npm, whose shell swallows the signal a supervisor sends npx, the launcher
notices its parent going and stops the server itself. Up to 3.1.0 it did none
of the signal half: it waited in spawnSync, where no handler can run, so a
SIGTERM ended the launcher and left the server running.

Each case lays out the launcher in a node_modules tree beside a platform
package whose binary is a stand-in shell script. The stand-in records its own
PID and its parent's, the arguments it was given and every signal it traps, so
the cases read what the server saw rather than what the launcher printed.

npm/gitlab-mcp-server/cli.js is POSIX-specific in what these cases hold:
on Windows it forwards nothing and watches nothing, which no case here can
run, so the module is skipped there.

Run with:

    python3 -m unittest discover -s scripts -p 'npm_cli_js_test.py'
"""

import os
import shutil
import signal
import subprocess
import tempfile
import time
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
LAUNCHER = os.path.join(ROOT, "npm", "gitlab-mcp-server", "cli.js")

# A stand-in for the server binary. It writes its PID and its parent's, then
# one line per argument and one per signal it traps, to the files the case
# names, and behaves as STUB_MODE says: exit with a code, die by a signal, or
# wait until a signal arrives.
STUB = """#!/bin/sh
printf '%s %s\\n' "$$" "$PPID" > "$STUB_PIDS.tmp" && mv "$STUB_PIDS.tmp" "$STUB_PIDS"
for a in "$@"; do
  printf 'arg=[%s]\\n' "$a" >> "$STUB_LOG"
done
trap 'echo TERM >> "$STUB_LOG"; exit 0' TERM
trap 'echo INT >> "$STUB_LOG"; exit 0' INT
trap 'echo HUP >> "$STUB_LOG"; exit 0' HUP
case "$STUB_MODE" in
  exit)
    printf 'out from the server\\n'
    exit "$STUB_CODE"
    ;;
  signal)
    trap - "$STUB_SIGNAL"
    kill -s "$STUB_SIGNAL" $$
    ;;
  wait)
    while :; do sleep 0.05; done
    ;;
esac
"""

# How long a case waits for something the launcher should do at once, and for
# the parent watch, which checks once a second.
SOON = 5.0
WATCH_PERIOD = 1.0


def node_key():
    """The platform key the launcher computes from process.platform and arch."""
    return subprocess.run(
        ["node", "-p", "process.platform + '-' + process.arch"],
        capture_output=True, text=True, check=True,
    ).stdout.strip()


def wait_for(cond, timeout):
    """Polls cond until it is true or timeout seconds pass."""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if cond():
            return True
        time.sleep(0.02)
    return cond()


def alive(pid):
    """False for a process that is gone or waits only to be reaped."""
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    # The process can be reaped between the signal probe and the read, which
    # removes its /proc entry; that is a process that is gone. Opening the file
    # rather than testing for it first is what keeps the two from racing.
    try:
        with open("/proc/{}/stat".format(pid), encoding="utf-8") as fh:
            text = fh.read()
    except FileNotFoundError:
        return not os.path.isdir("/proc/self")
    return text[text.rindex(")") + 2] != "Z"


@unittest.skipUnless(os.name == "posix", "the launcher forwards signals on POSIX only")
class NpmLauncherTest(unittest.TestCase):
    """Runs the real cli.js against a stand-in server binary."""

    def setUp(self):
        if shutil.which("node") is None:
            reason = "the launcher needs node"
            # GitHub sets CI on every step; there a missing tool is a failure.
            if os.environ.get("CI"):
                self.fail(reason + ", and CI must run these cases rather than skip them")
            self.skipTest(reason)
        self.work = tempfile.mkdtemp(prefix="npm-cli-js-")
        self.addCleanup(shutil.rmtree, self.work, True)
        modules = os.path.join(self.work, "node_modules", "@jmrp.io")
        self.launcher_dir = os.path.join(modules, "gitlab-mcp-server")
        os.makedirs(self.launcher_dir)
        shutil.copyfile(LAUNCHER, os.path.join(self.launcher_dir, "cli.js"))
        self.platform_dir = os.path.join(modules, "gitlab-mcp-server-" + node_key())
        os.makedirs(self.platform_dir)
        self.binary = os.path.join(self.platform_dir, "gitlab-mcp-server")
        with open(self.binary, "w", encoding="utf-8") as fh:
            fh.write(STUB)
        os.chmod(self.binary, 0o755)
        self.log = os.path.join(self.work, "log")
        self.pids = os.path.join(self.work, "pids")
        open(self.log, "w", encoding="utf-8").close()
        self.cli = os.path.join(self.launcher_dir, "cli.js")

    def env(self, mode, npm=False, **extra):
        """A clean environment: no npm variables unless the case asks for them."""
        env = {k: v for k, v in os.environ.items() if not k.startswith("npm_")}
        env.update(STUB_MODE=mode, STUB_LOG=self.log, STUB_PIDS=self.pids)
        if npm:
            env["npm_lifecycle_event"] = "npx"
        env.update(extra)
        return env

    def logged(self):
        with open(self.log, encoding="utf-8") as fh:
            return fh.read().splitlines()

    def stub_pids(self):
        """The stand-in's PID and its parent's, once it has written them."""
        self.assertTrue(wait_for(lambda: os.path.exists(self.pids), SOON), "the stand-in server never started")
        with open(self.pids, encoding="utf-8") as fh:
            server, parent = (int(field) for field in fh.read().split())
        self.addCleanup(self.kill_quietly, server)
        self.addCleanup(self.kill_quietly, parent)
        return server, parent

    @staticmethod
    def kill_quietly(pid):
        try:
            os.kill(pid, signal.SIGKILL)
        except ProcessLookupError:
            pass

    def start(self, mode, **kwargs):
        proc = subprocess.Popen(
            ["node", self.cli, "--http", "a b", ""],
            env=self.env(mode, **kwargs), stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True,
        )
        self.addCleanup(self.reap, proc)
        return proc

    @staticmethod
    def reap(proc):
        if proc.poll() is None:
            proc.kill()
        proc.communicate()

    def test_passes_arguments_and_stdout_through_and_mirrors_the_exit_code(self):
        result = subprocess.run(
            ["node", self.cli, "--http", "a b", ""], env=self.env("exit", STUB_CODE="7"),
            capture_output=True, text=True, timeout=SOON * 2, check=False,
        )
        self.assertEqual(result.returncode, 7, result.stderr)
        self.assertEqual(result.stdout, "out from the server\n")
        self.assertEqual(result.stderr, "")
        self.assertEqual(self.logged(), ["arg=[--http]", "arg=[a b]", "arg=[]"])

    def test_dies_by_the_signal_that_ended_the_server(self):
        # SIGTERM is one the launcher listens for, so the case fails if the
        # launcher re-raises it without first removing its own handler: it
        # would catch the signal and exit 143 instead of dying by it.
        for name in ("TERM", "KILL"):
            with self.subTest(name):
                result = subprocess.run(
                    ["node", self.cli], env=self.env("signal", STUB_SIGNAL=name),
                    capture_output=True, text=True, timeout=SOON * 2, check=False,
                )
                self.assertEqual(result.returncode, -getattr(signal, "SIG" + name), result.stderr)

    def test_passes_each_signal_on_to_the_server(self):
        for name in ("TERM", "INT", "HUP"):
            with self.subTest(name):
                open(self.log, "w", encoding="utf-8").close()
                if os.path.exists(self.pids):
                    os.remove(self.pids)
                proc = self.start("wait")
                server, launcher = self.stub_pids()
                self.assertEqual(launcher, proc.pid)
                os.kill(proc.pid, getattr(signal, "SIG" + name))
                proc.wait(timeout=SOON)
                self.assertEqual(proc.returncode, 0)
                self.assertIn(name, self.logged())
                self.assertFalse(alive(server))

    def shell_around_the_launcher(self, npm):
        """Runs the launcher under a shell that does not exec it, as npm does."""
        script = "node \"$0\"; :"
        proc = subprocess.Popen(
            ["/bin/sh", "-c", script, self.cli],
            env=self.env("wait", npm=npm), stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True,
        )
        self.addCleanup(self.reap, proc)
        server, launcher = self.stub_pids()
        return proc, server, launcher

    def test_stops_the_server_when_the_shell_npm_runs_it_through_dies(self):
        shell, server, launcher = self.shell_around_the_launcher(npm=True)
        shell.send_signal(signal.SIGTERM)
        shell.wait(timeout=SOON)
        self.assertTrue(
            wait_for(lambda: not alive(server), WATCH_PERIOD + SOON),
            "the server outlived the shell it was started under",
        )
        self.assertIn("TERM", self.logged())
        self.assertTrue(wait_for(lambda: not alive(launcher), SOON), "the launcher outlived its server")

    def test_leaves_the_server_running_when_not_started_by_npm(self):
        # Without npm's variables the parent is the client or a shell, and a
        # server deliberately left running after it is not the launcher's to
        # stop: the same server started directly would keep running too.
        shell, server, launcher = self.shell_around_the_launcher(npm=False)
        shell.send_signal(signal.SIGTERM)
        shell.wait(timeout=SOON)
        time.sleep(WATCH_PERIOD * 2.5)
        self.assertTrue(alive(server), "the server was stopped although nothing asked")
        self.assertNotIn("TERM", self.logged())
        os.kill(launcher, signal.SIGTERM)
        self.assertTrue(wait_for(lambda: not alive(server), SOON), "a SIGTERM to the launcher did not reach the server")

    def test_reports_a_binary_it_cannot_start(self):
        os.chmod(self.binary, 0o644)
        result = subprocess.run(
            ["node", self.cli], env=self.env("exit", STUB_CODE="0"),
            capture_output=True, text=True, timeout=SOON * 2, check=False,
        )
        self.assertEqual(result.returncode, 1)
        self.assertIn("gitlab-mcp-server: failed to start the binary:", result.stderr)

    def test_reports_a_platform_package_that_is_not_installed(self):
        shutil.rmtree(self.platform_dir)
        result = subprocess.run(
            ["node", self.cli], env=self.env("exit", STUB_CODE="0"),
            capture_output=True, text=True, timeout=SOON * 2, check=False,
        )
        self.assertEqual(result.returncode, 1)
        self.assertIn("package is not installed", result.stderr)


if __name__ == "__main__":
    unittest.main()
