#!/usr/bin/env python3
"""Tests scripts/ensure-playwright-chromium.sh, the Chromium step of both site builds.

The site build renders its Mermaid diagrams with Playwright's Chromium, and
the step that prepared it used to be `playwright install --with-deps
chromium`, which ran apt-get update and install on every run. On 2026-10-07
an apt run that never ended took three CI site build jobs to their 20-minute
timeout with every cache hit. The script installs the browser alone, launches
it, and goes to apt (through `playwright install-deps`) only when the launch
fails, each attempt under timeout(1).

These cases drive the real script against a stand-in `pnpm`, `node` and
`sudo` placed first on PATH. The stand-in pnpm answers `playwright install
chromium` as each case tells it to (succeed, fail, or hang until something
kills it). The stand-in node is both the launch check and Playwright's CLI:
as the CLI it answers `install-deps chromium` the same way, and an
install-deps that succeeds leaves a marker standing for the libraries it
installed; as the launch check it launches when the case says so or once that
marker exists. The stand-in sudo records what it was asked to run and runs it,
the stand-in dpkg records the repair the script runs before a retry, and `apt`
and `apt-get` are tripwires, because nothing but install-deps may reach apt.
What is asserted is which calls were made, in what
order and as whom, the exit status, the annotations a run prints, and how long
a run whose apt hangs takes to fail. The bounds are shortened through the
variables the script reads, and the defaults CI uses are held against the
step caps the two workflows give the step.

Run with:

    python3 -m unittest discover -s scripts -p 'ensure_playwright_chromium_sh_test.py'
"""

import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import time
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
SCRIPT = os.path.join(ROOT, "scripts", "ensure-playwright-chromium.sh")
WORKFLOWS = {
    "ci.yml": ("site-build", os.path.join(ROOT, ".github", "workflows", "ci.yml")),
    "pages.yml": ("build", os.path.join(ROOT, ".github", "workflows", "pages.yml")),
}

# Stands in for pnpm. Only `pnpm exec playwright install chromium` is
# expected; anything else is a failure the case reports. STUB_BROWSER is ok,
# fail or hang.
STUB_PNPM = r'''import json
import os
import sys
import time

args = sys.argv[1:]
state = os.environ["STUB_STATE"]
with open(os.path.join(state, "calls.jsonl"), "a", encoding="utf-8") as fh:
    fh.write(json.dumps({"tool": "pnpm", "argv": args, "cwd": os.getcwd()}) + "\n")
if args != ["exec", "playwright", "install", "chromium"]:
    sys.exit("unexpected pnpm call: " + " ".join(args))
behaviour = os.environ.get("STUB_BROWSER", "ok")
if behaviour == "hang":
    time.sleep(120)
sys.exit(0 if behaviour == "ok" else 1)
'''

# Stands in for node, in its two roles. Run with `--input-type=module -e` it
# is the launch check: STUB_LAUNCH is always, never, hang, or libraries (the
# default), which launches once install-deps has succeeded. Run on
# node_modules/playwright/cli.js it is Playwright's CLI, and only
# `install-deps chromium` is expected: STUB_DEPS is a comma-separated plan,
# one entry per attempt (the last repeats), each ok, fail or hang, and an
# install-deps that succeeds creates STUB_STATE/libraries.
STUB_NODE = r'''import json
import os
import sys
import time

args = sys.argv[1:]
state = os.environ["STUB_STATE"]
with open(os.path.join(state, "calls.jsonl"), "a", encoding="utf-8") as fh:
    fh.write(json.dumps({"tool": "node", "argv": args, "cwd": os.getcwd()}) + "\n")
if args[:2] == ["--input-type=module", "-e"]:
    launch = os.environ.get("STUB_LAUNCH", "libraries")
    if launch == "hang":
        time.sleep(120)
    if launch == "always" or (launch == "libraries" and os.path.exists(os.path.join(state, "libraries"))):
        sys.exit(0)
    print("browserType.launch: error while loading shared libraries: libglib-2.0.so.0", file=sys.stderr)
    sys.exit(1)
if args != ["node_modules/playwright/cli.js", "install-deps", "chromium"]:
    sys.exit("unexpected node call: " + " ".join(args))
counter = os.path.join(state, "deps-attempts")
attempt = 1
if os.path.exists(counter):
    with open(counter, encoding="utf-8") as fh:
        attempt = int(fh.read()) + 1
with open(counter, "w", encoding="utf-8") as fh:
    fh.write(str(attempt))
plan = os.environ.get("STUB_DEPS", "ok").split(",")
behaviour = plan[min(attempt, len(plan)) - 1]
if behaviour == "hang":
    time.sleep(120)
if behaviour != "ok":
    sys.exit(1)
open(os.path.join(state, "libraries"), "w", encoding="utf-8").close()
'''

# Stands in for sudo: records what it was asked to run, and runs it. The real
# one runs it as root and passes a signal on to that one process and to
# nothing it starts, and a timeout running as the user cannot signal root's
# processes, which is why the script hands sudo the timeout and not the other
# way round.
STUB_SUDO = r'''import json
import os
import sys

args = sys.argv[1:]
state = os.environ["STUB_STATE"]
with open(os.path.join(state, "calls.jsonl"), "a", encoding="utf-8") as fh:
    fh.write(json.dumps({"tool": "sudo", "argv": args, "cwd": os.getcwd()}) + "\n")
if args[:1] == ["--"]:
    args = args[1:]
os.execvp(args[0], args)
'''

# Anything that reaches apt other than through install-deps.
STUB_TRIPWIRE = r'''import json
import os
import sys

state = os.environ["STUB_STATE"]
with open(os.path.join(state, "calls.jsonl"), "a", encoding="utf-8") as fh:
    fh.write(json.dumps({"tool": os.path.basename(sys.argv[0]), "argv": sys.argv[1:]}) + "\n")
sys.exit(97)
'''

# Stands in for dpkg, which the script runs as `dpkg --configure -a` before a
# retry, to finish what a stopped attempt left half done. STUB_DPKG is ok,
# fail or hang.
STUB_DPKG = r'''import json
import os
import sys
import time

args = sys.argv[1:]
state = os.environ["STUB_STATE"]
with open(os.path.join(state, "calls.jsonl"), "a", encoding="utf-8") as fh:
    fh.write(json.dumps({"tool": "dpkg", "argv": args, "cwd": os.getcwd()}) + "\n")
if args != ["--configure", "-a"]:
    sys.exit("unexpected dpkg call: " + " ".join(args))
behaviour = os.environ.get("STUB_DPKG", "ok")
if behaviour == "hang":
    time.sleep(120)
sys.exit(0 if behaviour == "ok" else 2)
'''

# Stands in for id, so that a case decides whether the script believes it is
# root. A GitHub-hosted runner is not; a container may be.
STUB_ID = r'''import os
import sys

if sys.argv[1:] != ["-u"]:
    sys.exit("unexpected id call: " + " ".join(sys.argv[1:]))
print(os.environ.get("STUB_UID", "1001"))
'''

INSTALL_BROWSER = ["exec", "playwright", "install", "chromium"]
INSTALL_DEPS = ["node_modules/playwright/cli.js", "install-deps", "chromium"]


def script_default(name):
    """Returns the default the script gives one of its bounds."""
    with open(SCRIPT, encoding="utf-8") as fh:
        text = fh.read()
    match = re.search(r"\$\{" + name + r":-(\d+)\}", text)
    if match is None:
        raise AssertionError(name + " has no default in the script")
    return int(match.group(1))


def kill_after(variable):
    """Returns the --kill-after of the timeout the script bounds by a variable."""
    with open(SCRIPT, encoding="utf-8") as fh:
        text = fh.read()
    found = re.findall(r"timeout --kill-after=(\d+) \"\$" + variable + "\"", text)
    if len(found) != 1:
        raise AssertionError("expected one timeout bounded by $" + variable + ", found " + str(len(found)))
    return int(found[0])


class EnsurePlaywrightChromiumTest(unittest.TestCase):
    def setUp(self):
        self.scratch = tempfile.mkdtemp(prefix="ensure-chromium-")
        self.addCleanup(shutil.rmtree, self.scratch, ignore_errors=True)
        self.site = os.path.join(self.scratch, "site")
        self.state = os.path.join(self.scratch, "state")
        self.bin = os.path.join(self.scratch, "bin")
        for path in (self.site, self.state, self.bin):
            os.mkdir(path)
        shebang = "#!" + sys.executable + " -S\n"
        stubs = {"pnpm": STUB_PNPM, "node": STUB_NODE, "sudo": STUB_SUDO, "id": STUB_ID, "dpkg": STUB_DPKG}
        for name in ("apt", "apt-get"):
            stubs[name] = STUB_TRIPWIRE
        for name, body in stubs.items():
            path = os.path.join(self.bin, name)
            with open(path, "w", encoding="utf-8") as fh:
                fh.write(shebang + body)
            os.chmod(path, 0o755)

    def run_script(self, env=None):
        """Runs the script from the stand-in site directory; returns (proc, calls, seconds)."""
        environment = dict(os.environ)
        for key in list(environment):
            if key.startswith(("PLAYWRIGHT_", "STUB_")):
                del environment[key]
        environment.update({
            "PATH": self.bin + os.pathsep + environment.get("PATH", ""),
            "STUB_STATE": self.state,
            # Short enough that a case whose stand-in hangs ends in seconds.
            "PLAYWRIGHT_BROWSER_SECONDS": "2",
            "PLAYWRIGHT_LAUNCH_SECONDS": "2",
            "PLAYWRIGHT_DEPS_ATTEMPT_SECONDS": "2",
            "PLAYWRIGHT_DPKG_REPAIR_SECONDS": "2",
        })
        environment.update(env or {})
        started = time.monotonic()
        proc = subprocess.run([SCRIPT], cwd=self.site, env=environment, capture_output=True,
                              text=True, timeout=120, check=False)
        seconds = time.monotonic() - started
        calls = []
        log = os.path.join(self.state, "calls.jsonl")
        if os.path.exists(log):
            with open(log, encoding="utf-8") as fh:
                calls = [json.loads(line) for line in fh]
        return proc, calls, seconds

    def sequence(self, calls):
        """Names each call: browser, launch, sudo, deps, or the tripwire that fired."""
        names = []
        for call in calls:
            if call["tool"] == "pnpm" and call["argv"] == INSTALL_BROWSER:
                names.append("browser")
            elif call["tool"] == "node" and call["argv"] == INSTALL_DEPS:
                names.append("deps")
            elif call["tool"] == "node" and call["argv"][:2] == ["--input-type=module", "-e"]:
                names.append("launch")
            elif call["tool"] == "sudo":
                names.append("sudo")
            elif call["tool"] == "dpkg" and call["argv"] == ["--configure", "-a"]:
                names.append("repair")
            else:
                names.append(call["tool"] + " " + " ".join(call["argv"]))
        return names

    def test_browser_that_launches_is_left_alone_and_apt_is_never_reached(self):
        proc, calls, _ = self.run_script({"STUB_LAUNCH": "always"})
        output = proc.stdout + proc.stderr
        self.assertEqual(proc.returncode, 0, output)
        self.assertEqual(self.sequence(calls), ["browser", "launch"])
        self.assertIn("apt was not used", output)
        self.assertNotIn("::warning", output)
        for call in calls:
            self.assertEqual(call["cwd"], self.site)

    def test_install_deps_runs_as_root_inside_its_timeout(self):
        # Playwright hands sudo `sh -c "apt-get update&& ..."`, and sudo
        # passes a signal on to that shell and to nothing it starts, while a
        # timeout running as the runner's user cannot signal root's apt-get
        # itself; so a timeout outside sudo left apt-get holding apt's lock
        # against the next attempt. The timeout goes inside sudo, around
        # Playwright's CLI, with node named by its path because sudo replaces
        # PATH.
        proc, calls, _ = self.run_script()
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assertEqual(self.sequence(calls), ["browser", "launch", "sudo", "deps", "launch"])
        sudo = [call for call in calls if call["tool"] == "sudo"][0]
        self.assertEqual(sudo["argv"], ["--", "timeout", "--kill-after=15", "2",
                                        os.path.join(self.bin, "node")] + INSTALL_DEPS)
        self.assertEqual(sudo["cwd"], self.site)

    def test_install_deps_as_root_needs_no_sudo(self):
        proc, calls, _ = self.run_script({"STUB_UID": "0"})
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assertEqual(self.sequence(calls), ["browser", "launch", "deps", "launch"])

    def test_launch_check_launches_the_way_rehype_mermaid_does(self):
        # mermaid-isomorphic, which rehype-mermaid renders through, calls
        # chromium.launch() with no options on the playwright package the
        # site depends on; the check is that call, from the site directory.
        _, calls, _ = self.run_script({"STUB_LAUNCH": "always"})
        launch = [call for call in calls if call["tool"] == "node"][0]
        self.assertEqual(launch["argv"][:2], ["--input-type=module", "-e"])
        program = launch["argv"][2]
        self.assertIn('import { chromium } from "playwright";', program)
        self.assertIn("await chromium.launch();", program)
        self.assertIn("await browser.newPage();", program)
        self.assertIn("process.exit(1);", program)

    def test_browser_that_does_not_launch_gets_its_libraries_and_is_launched_again(self):
        proc, calls, _ = self.run_script()
        output = proc.stdout + proc.stderr
        self.assertEqual(proc.returncode, 0, output)
        self.assertEqual(self.sequence(calls), ["browser", "launch", "sudo", "deps", "launch"])
        self.assertIn("::warning title=Playwright Chromium::Chromium does not launch", output)
        self.assertIn("libglib-2.0.so.0", output)
        self.assertIn("Chromium launches after its system libraries were installed.", output)

    def test_failed_attempt_is_retried(self):
        proc, calls, _ = self.run_script({"STUB_DEPS": "fail,ok"})
        output = proc.stdout + proc.stderr
        self.assertEqual(proc.returncode, 0, output)
        self.assertEqual(self.sequence(calls),
                         ["browser", "launch", "sudo", "deps", "sudo", "repair", "sudo", "deps", "launch"])
        self.assertIn("Attempt 1 exited with status 1.", output)

    def test_retry_finishes_what_dpkg_was_left_doing_first(self):
        # An attempt stopped while dpkg was unpacking leaves dpkg marked
        # interrupted, and apt-get refuses to run until `dpkg --configure -a`
        # has finished the job. A plain Ubuntu image on a slow link reached
        # that state and failed its second attempt at once. The repair runs as
        # root, inside a bound of its own, before every retry and never before
        # the first attempt.
        proc, calls, _ = self.run_script({"STUB_DEPS": "hang,ok", "PLAYWRIGHT_DPKG_REPAIR_SECONDS": "3"})
        output = proc.stdout + proc.stderr
        self.assertEqual(proc.returncode, 0, output)
        self.assertEqual(self.sequence(calls),
                         ["browser", "launch", "sudo", "deps", "sudo", "repair", "sudo", "deps", "launch"])
        repair = [call for call in calls if call["tool"] == "sudo"][1]
        self.assertEqual(repair["argv"], ["--", "timeout", "--kill-after=10", "3", "dpkg", "--configure", "-a"])
        self.assertIn("dpkg --configure -a, to finish what attempt 1 may have left half done", output)

    def test_repair_that_fails_or_hangs_does_not_stop_the_retry(self):
        # apt-get says what is wrong when the repair could not make it right,
        # so a repair that fails is reported and the retry goes ahead.
        for behaviour in ("fail", "hang"):
            with self.subTest(behaviour):
                for name in ("calls.jsonl", "deps-attempts", "libraries"):
                    path = os.path.join(self.state, name)
                    if os.path.exists(path):
                        os.remove(path)
                proc, calls, seconds = self.run_script({"STUB_DEPS": "fail,ok", "STUB_DPKG": behaviour,
                                                        "PLAYWRIGHT_DPKG_REPAIR_SECONDS": "1"})
                output = proc.stdout + proc.stderr
                self.assertEqual(proc.returncode, 0, output)
                self.assertEqual(self.sequence(calls).count("deps"), 2)
                self.assertRegex(output, r"dpkg --configure -a exited with status (2|124)\.")
                self.assertLess(seconds, 30, output)

    def test_apt_that_hangs_fails_within_the_bound_with_the_reason(self):
        proc, calls, seconds = self.run_script({"STUB_DEPS": "hang"})
        output = proc.stdout + proc.stderr
        self.assertEqual(proc.returncode, 1, output)
        self.assertEqual(self.sequence(calls), ["browser", "launch", "sudo", "deps", "sudo", "repair", "sudo", "deps"])
        self.assertIn("Attempt 1 was stopped after 2 s.", output)
        self.assertIn("Attempt 2 was stopped after 2 s.", output)
        self.assertIn("::error title=Playwright Chromium::Chromium does not launch, and its system "
                      "libraries could not be installed with apt (2 attempts of at most 2 s each).", output)
        # Two attempts of 2 s each and the launch checks; the stand-in would
        # sleep 120 s per attempt if nothing ended it.
        self.assertLess(seconds, 30, output)

    def test_attempts_are_configurable(self):
        proc, calls, _ = self.run_script({"STUB_DEPS": "fail", "PLAYWRIGHT_DEPS_ATTEMPTS": "3"})
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertEqual(self.sequence(calls).count("deps"), 3)

    def test_libraries_installed_and_still_no_launch_is_not_retried(self):
        # A second install-deps would install nothing the first did not. The
        # error says apt succeeded, because the reader's next question is why
        # the browser refuses to start, not why apt failed.
        proc, calls, _ = self.run_script({"STUB_LAUNCH": "never"})
        output = proc.stdout + proc.stderr
        self.assertEqual(proc.returncode, 1, output)
        self.assertEqual(self.sequence(calls), ["browser", "launch", "sudo", "deps", "launch"])
        self.assertIn("::warning title=Playwright Chromium::Chromium does not launch with the libraries "
                      "this machine has", output)
        self.assertIn("::error title=Playwright Chromium::playwright install-deps chromium installed "
                      "Chromium's system libraries, and Chromium still does not launch.", output)
        self.assertNotIn("could not be installed", output)
        self.assertEqual(output.count("::error"), 1, output)

    def test_launch_that_hangs_counts_as_a_launch_that_failed(self):
        # A launch that runs out of time is said to have run out of time,
        # not to have failed for want of a library.
        proc, calls, seconds = self.run_script({"STUB_LAUNCH": "hang"})
        output = proc.stdout + proc.stderr
        self.assertEqual(proc.returncode, 1, output)
        self.assertEqual(self.sequence(calls), ["browser", "launch", "sudo", "deps", "launch"])
        self.assertIn("::warning title=Playwright Chromium::Chromium did not launch within 2 s with the "
                      "libraries this machine has", output)
        self.assertIn("::error title=Playwright Chromium::playwright install-deps chromium installed "
                      "Chromium's system libraries, and Chromium still did not launch within 2 s. The launch "
                      "check's output, if it printed any, is above.", output)
        self.assertNotIn("could not be installed", output)
        self.assertEqual(output.count("::error"), 1, output)
        self.assertLess(seconds, 30, output)

    def test_browser_install_that_fails_or_hangs_stops_before_the_launch(self):
        for behaviour in ("fail", "hang"):
            with self.subTest(behaviour):
                log = os.path.join(self.state, "calls.jsonl")
                if os.path.exists(log):
                    os.remove(log)
                proc, calls, seconds = self.run_script({"STUB_BROWSER": behaviour, "STUB_LAUNCH": "always"})
                output = proc.stdout + proc.stderr
                self.assertEqual(proc.returncode, 1, output)
                self.assertEqual(self.sequence(calls), ["browser"])
                self.assertIn("::error title=Playwright Chromium::playwright install chromium did not "
                              "finish within 2 s", output)
                self.assertLess(seconds, 30, output)

    def test_workflows_run_the_script_capped_above_its_own_bounds(self):
        # The step's timeout-minutes has to be above the longest the script
        # can take with its CI defaults, every bound reached at once, so that
        # a failure ends with the script's message rather than the runner's
        # cancellation; and the job's above the step's.
        browser = script_default("PLAYWRIGHT_BROWSER_SECONDS") + kill_after("browser_seconds")
        launch = script_default("PLAYWRIGHT_LAUNCH_SECONDS") + kill_after("launch_seconds")
        attempt = script_default("PLAYWRIGHT_DEPS_ATTEMPT_SECONDS") + kill_after("attempt_seconds")
        repair = script_default("PLAYWRIGHT_DPKG_REPAIR_SECONDS") + kill_after("repair_seconds")
        attempts = script_default("PLAYWRIGHT_DEPS_ATTEMPTS")
        worst = browser + launch + attempts * attempt + (attempts - 1) * repair + launch
        for name, (job, path) in WORKFLOWS.items():
            with self.subTest(name):
                with open(path, encoding="utf-8") as fh:
                    text = fh.read()
                # The comments name the flag to say why it went; no command
                # may pass it.
                self.assertNotRegex(text, r"(?m)^[^#\n]*playwright install --with-deps")
                block = re.search(r"^  " + re.escape(job) + r":\n((?:    .*\n|\n)+)", text, re.MULTILINE)
                self.assertIsNotNone(block, "job " + job + " is missing")
                body = block.group(1)
                job_cap = re.search(r"^    timeout-minutes: (\d+)$", body, re.MULTILINE)
                step = re.search(r"^      - name: Install Playwright Chromium\n((?:        .*\n)+)", body, re.MULTILINE)
                self.assertIsNotNone(job_cap, "job " + job + " has no timeout-minutes")
                self.assertIsNotNone(step, "the Chromium step is missing from " + job)
                step_body = step.group(1)
                self.assertIn("run: ../scripts/ensure-playwright-chromium.sh\n", step_body)
                self.assertIn("working-directory: site\n", step_body)
                step_cap = re.search(r"^        timeout-minutes: (\d+)$", step_body, re.MULTILINE)
                self.assertIsNotNone(step_cap, "the Chromium step has no timeout-minutes")
                self.assertGreater(int(step_cap.group(1)) * 60, worst)
                self.assertGreater(int(job_cap.group(1)), int(step_cap.group(1)))

    def test_script_passes_shellcheck(self):
        shellcheck = shutil.which("shellcheck")
        if shellcheck is None:
            # A skip keeps the job green, so on a runner without shellcheck
            # this check would stop running and nobody would be told.
            # GitHub sets CI on every step; there a missing tool is a failure.
            if os.environ.get("CI"):
                self.fail("shellcheck is not installed, and CI must run this check rather than skip it")
            self.skipTest("shellcheck is not installed")
        result = subprocess.run([shellcheck, SCRIPT], capture_output=True, timeout=60, check=False)
        self.assertEqual(result.returncode, 0, result.stdout.decode() + result.stderr.decode())


if __name__ == "__main__":
    unittest.main()
