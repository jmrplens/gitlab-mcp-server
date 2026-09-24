#!/usr/bin/env python3
"""Tests scripts/coverage-mutants.sh, the recipe behind `make coverage-mutants`.

The script decides how long gremlins gives each mutant. gremlins multiplies
--timeout-coefficient by the wall time of its own coverage run, the
`go test [-tags T] [-coverpkg P] -cover -coverprofile F ./<pkg>/...` it runs
from the module root, so the coefficient is only right when the baseline the
script derives it from measures that same run. It did not (issue 915). The
baseline ran without the tags gremlins was given, so a package whose files
all carry `//go:build e2e` except its doc.go printed `[no test files]`, the
parser found no duration on that line, and a 0.010 s fallback turned into a
coefficient of 3001 and a deadline of 95 hours per mutant. A parsed duration
was wrong too, being the test binary's own run time without the build.

These cases drive the real script against a stand-in `go` placed first on
PATH, which answers the `go list`, `go mod download` and `go test` calls from
what each case configures, records every call, and records the gremlins run
instead of starting it. What is asserted is what reaches gremlins: the tags
every go command before it carries, the command the baseline times, the
coefficient derived from the printed base, the ceiling on the deadline, and
the refusals that stop a run whose figures would mean nothing.

Run with:

    python3 -m unittest discover -s scripts -p 'coverage_mutants_sh_test.py'
"""

import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
SCRIPT = os.path.join(ROOT, "scripts", "coverage-mutants.sh")

# Long enough that a baseline which really ran the stand-in's `go test` is
# told apart from one that did not, and short enough that the whole file
# stays in seconds: every successful case runs the baseline twice.
SLEEP = 0.3

# The script's own defaults, which the Makefile passes as MUTANT_BUDGET and
# MUTANT_DEADLINE_MAX.
DEFAULT_BUDGET = 300
DEFAULT_CEILING = 3600

# Stands in for the go command. Configured through STUB_* variables, which
# the script passes through untouched, and appends one JSON line per call to
# STUB_LOG. Its interpreter line is written in setUp, naming the interpreter
# running these tests with -S: an `env python3` can land on a version manager's
# shim, and site initialisation can import whatever .pth files an environment
# carries, each costing most of a second that every go call would then pay and
# the baseline would then time. The stand-in needs the standard library only.
STUB_GO = r'''import json
import os
import sys
import time

args = sys.argv[1:]
env = os.environ


def tags_of(argv):
    tags = []
    for i, arg in enumerate(argv):
        if arg == "-tags" and i + 1 < len(argv):
            tags = argv[i + 1].split(",")
        elif arg.startswith("-tags="):
            tags = arg[len("-tags="):].split(",")
    return tags


def tagged(argv):
    need = env.get("STUB_NEEDS_TAG", "")
    return not need or need in tags_of(argv)


def render(template, fields):
    for placeholder, value in fields.items():
        template = template.replace(placeholder, value)
    if "{{" in template:
        sys.stderr.write("stub go list cannot render " + template + "\n")
        sys.exit(2)
    return template


entry = {"argv": args, "cwd": os.getcwd(), "goflags": env.get("GOFLAGS", "")}
try:
    entry["stdout"] = os.readlink("/proc/self/fd/1")
except OSError:
    entry["stdout"] = None

status = 0
if args[:2] == ["list", "-m"]:
    print(render(args[args.index("-f") + 1], {"{{.Dir}}": env["STUB_ROOT"]}))
elif args[:1] == ["list"]:
    pkgdir = os.path.normpath(os.path.join(os.getcwd(), args[-1]))
    name = env.get("STUB_PKG_NAME") or os.path.basename(pkgdir)
    counts = env.get("STUB_TEST_FILES", "3 0") if tagged(args) else "0 0"
    error = ""
    if not tagged(args) and env.get("STUB_ALL_TAGGED"):
        name = ""
        error = "package example.com/m/%s: build constraints exclude all Go files in %s" % (
            os.path.basename(pkgdir), pkgdir)
    if error and "-e" not in args:
        sys.stderr.write(error + "\n")
        status = 1
    else:
        tests, xtests = counts.split()
        print(render(args[args.index("-f") + 1], {
            "{{.Name}}": name,
            "{{.Dir}}": pkgdir,
            "{{len .TestGoFiles}}": tests,
            "{{len .XTestGoFiles}}": xtests,
            "{{with .Error}}{{.}}{{end}}": error,
        }))
elif args[:2] == ["mod", "download"]:
    pass
elif args[:1] == ["test"]:
    pattern = args[-1]
    entry["pattern_dir_exists"] = os.path.isdir(os.path.join(os.getcwd(), pattern.rstrip(".").rstrip("/")))
    if not tagged(args):
        print("?   \t%s\t[no test files]" % pattern)
    else:
        with open(env["STUB_LOG"], encoding="utf-8") as fh:
            run = 1 + sum(1 for line in fh if json.loads(line)["argv"][:1] == ["test"])
        time.sleep(float(env.get("STUB_TEST_SLEEP", "0")))
        if "-coverprofile" in args:
            with open(args[args.index("-coverprofile") + 1], "w", encoding="utf-8") as fh:
                fh.write("mode: set\n")
        if env.get("STUB_TEST_FAIL_RUN") in (str(run), "all"):
            print("--- FAIL: TestPlanted (0.00s)")
            print("FAIL\t%s\t0.100s" % pattern)
            status = 1
        else:
            print(env.get("STUB_TEST_LAST_LINE",
                          "ok  \t%s\t0.912s\tcoverage: 71.4%% of statements" % pattern))
elif args[:2] == ["run", "github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0"]:
    pass
else:
    sys.stderr.write("stub go: unexpected call %r\n" % (args,))
    status = 2

entry["status"] = status
with open(env["STUB_LOG"], "a", encoding="utf-8") as fh:
    fh.write(json.dumps(entry) + "\n")
sys.exit(status)
'''


def expected_coefficient(budget, base, ceiling):
    """The coefficient the script is meant to derive, recomputed from the
    base it printed: the budget's multiple, between 8 and 6000, then held
    under the ceiling's."""
    coefficient = min(max(int(budget / base) + 1, 8), 6000)
    return min(coefficient, int(ceiling / base))


def comma_decimal_locale(scratch):
    """A locale whose decimal separator is a comma, with the LOCPATH it needs:
    an installed one when `locale -a` lists one, otherwise es_ES compiled into
    scratch with localedef. None when neither is possible."""
    if shutil.which("locale") is None:
        return None
    listed = subprocess.run(["locale", "-a"], capture_output=True, text=True, check=False).stdout
    for name in listed.split():
        probe = subprocess.run(["locale", "decimal_point"], capture_output=True, text=True, check=False,
                               env=dict(os.environ, LC_ALL=name))
        if probe.stdout.strip() == ",":
            return name, None
    localedef = shutil.which("localedef")
    if localedef is None:
        return None
    locpath = os.path.join(scratch, "locales")
    os.makedirs(locpath)
    subprocess.run([localedef, "-i", "es_ES", "-f", "UTF-8", os.path.join(locpath, "es_ES.UTF-8")],
                   capture_output=True, check=False)
    probe = subprocess.run(["locale", "decimal_point"], capture_output=True, text=True, check=False,
                           env=dict(os.environ, LC_ALL="es_ES.UTF-8", LOCPATH=locpath))
    if probe.stdout.strip() == ",":
        return "es_ES.UTF-8", locpath
    return None


class CoverageMutantsTest(unittest.TestCase):

    def setUp(self):
        self.scratch = tempfile.mkdtemp(prefix="coverage-mutants-")
        self.addCleanup(shutil.rmtree, self.scratch, True)
        self.root = os.path.join(self.scratch, "module")
        for pkg, clause in (("internal/pkg", "pkg"), ("cmd/tool", "main")):
            os.makedirs(os.path.join(self.root, pkg))
            with open(os.path.join(self.root, pkg, "doc.go"), "w", encoding="utf-8") as fh:
                fh.write("package %s\n" % clause)
        with open(os.path.join(self.root, "go.mod"), "w", encoding="utf-8") as fh:
            fh.write("module example.com/m\n\ngo 1.27\n")
        self.bin = os.path.join(self.scratch, "bin")
        os.makedirs(self.bin)
        stub = os.path.join(self.bin, "go")
        with open(stub, "w", encoding="utf-8") as fh:
            fh.write("#!" + sys.executable + " -S\n" + STUB_GO)
        os.chmod(stub, 0o755)
        self.log = os.path.join(self.scratch, "go.log")

    def run_script(self, *args, flags="", env=None):
        """Runs the script from the module root, the way make does, and
        returns the finished process with the go calls it made."""
        if os.path.exists(self.log):
            os.remove(self.log)
        open(self.log, "w", encoding="utf-8").close()
        run_env = {k: v for k, v in os.environ.items()
                   if not k.startswith("GREMLINS_") and k != "GOFLAGS" and not k.startswith("STUB_")}
        run_env.update({
            "PATH": self.bin + os.pathsep + os.environ.get("PATH", ""),
            "STUB_LOG": self.log,
            "STUB_ROOT": self.root,
            "GREMLINS_FLAGS": flags,
        })
        run_env.update(env or {})
        proc = subprocess.run([SCRIPT, *args], cwd=self.root, env=run_env,
                              capture_output=True, text=True, timeout=120, check=False)
        with open(self.log, encoding="utf-8") as fh:
            calls = [json.loads(line) for line in fh]
        return proc, calls

    @staticmethod
    def of(calls, *prefix):
        return [c for c in calls if c["argv"][:len(prefix)] == list(prefix)]

    @staticmethod
    def package_lists(calls):
        return [c for c in calls if c["argv"][:1] == ["list"] and c["argv"][1:2] != ["-m"]]

    @staticmethod
    def tags_of(argv):
        tags = None
        for i, arg in enumerate(argv):
            if arg == "-tags" and i + 1 < len(argv):
                tags = argv[i + 1]
        return tags

    def gremlins(self, proc, calls):
        runs = self.of(calls, "run")
        self.assertEqual(len(runs), 1, proc.stdout + proc.stderr)
        return runs[0]

    def coefficient(self, proc, calls):
        argv = self.gremlins(proc, calls)["argv"]
        return int(argv[argv.index("--timeout-coefficient") + 1])

    def printed_base(self, proc):
        match = re.search(r"takes ([0-9]+\.[0-9]+)s", proc.stdout)
        self.assertIsNotNone(match, proc.stdout + proc.stderr)
        return float(match.group(1))

    def printed_deadline(self, proc):
        match = re.search(r"about ([0-9]+(?:\.[0-9]+)?)s per mutant", proc.stdout)
        self.assertIsNotNone(match, proc.stdout + proc.stderr)
        return float(match.group(1))

    def assert_refused_before_gremlins(self, proc, calls):
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertEqual(self.of(calls, "run"), [], "gremlins was started")

    def test_baseline_without_a_duration_on_its_last_line_is_timed_by_the_clock(self):
        # A -cover run ends in a coverage figure, so the last field of its
        # last line is "statements" and carries no duration at all.
        proc, calls = self.run_script("./internal/pkg", env={"STUB_TEST_SLEEP": str(SLEEP)})
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        base = self.printed_base(proc)
        self.assertGreaterEqual(base, SLEEP)
        coefficient = self.coefficient(proc, calls)
        self.assertEqual(coefficient, expected_coefficient(DEFAULT_BUDGET, base, DEFAULT_CEILING))
        # The budget is what each mutant gets, give or take one multiple of
        # the base, rather than the thirty-fold or hundred-fold of it a
        # misread base used to hand gremlins.
        deadline = self.printed_deadline(proc)
        self.assertAlmostEqual(deadline, base * coefficient, delta=0.1)
        self.assertGreaterEqual(deadline, DEFAULT_BUDGET)
        self.assertLessEqual(deadline, DEFAULT_BUDGET + base + 0.1)

    def test_build_tags_given_to_gremlins_reach_every_go_command_before_it(self):
        # Every spelling pflag reads a value for -t/--tags in, including a
        # shorthand cluster behind a boolean one, and gremlins' own
        # environment binding, which a flag overrides.
        cases = [
            ("--tags e2e", {}),
            ("--tags=e2e", {}),
            ("-t e2e", {}),
            ("-t=e2e", {}),
            ("-te2e", {}),
            ("-dt e2e", {}),
            ("-dte2e", {}),
            ("-dt=e2e", {}),
            ("--tags other --tags e2e", {}),
            ("", {"GREMLINS_UNLEASH_TAGS": "e2e"}),
            ("--tags e2e", {"GREMLINS_UNLEASH_TAGS": "other"}),
        ]
        for flags, env in cases:
            with self.subTest(flags=flags, env=env):
                # The package the measured failure was about: every file
                # behind the tag but one, so an untagged run finds no tests.
                proc, calls = self.run_script("./internal/pkg", flags=flags, env=dict(
                    env, STUB_NEEDS_TAG="e2e", STUB_TEST_SLEEP=str(SLEEP)))
                self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
                lists = self.package_lists(calls)
                self.assertTrue(lists)
                for call in lists + self.of(calls, "test"):
                    self.assertEqual(self.tags_of(call["argv"]), "e2e", call["argv"])
                base = self.printed_base(proc)
                self.assertGreaterEqual(base, SLEEP)
                self.assertEqual(self.coefficient(proc, calls),
                                 expected_coefficient(DEFAULT_BUDGET, base, DEFAULT_CEILING))

    def test_package_with_no_test_files_under_its_tags_is_refused(self):
        cases = [
            # Tests exist only behind a tag nobody passed: the harness case.
            ("tagged tests, untagged run", {"STUB_NEEDS_TAG": "e2e"}, "no test files"),
            # No tests at all.
            ("no tests", {"STUB_TEST_FILES": "0 0"}, "no test files"),
            # Every file behind the tag: go list itself reports the package
            # unloadable, which must reach the reader as the same refusal.
            ("every file tagged", {"STUB_NEEDS_TAG": "e2e", "STUB_ALL_TAGGED": "1"},
             "build constraints exclude all Go files"),
        ]
        for name, env, says in cases:
            with self.subTest(name):
                proc, calls = self.run_script("./internal/pkg", env=env)
                self.assert_refused_before_gremlins(proc, calls)
                self.assertEqual(self.of(calls, "test"), [])
                self.assertIn(says, proc.stderr)
                self.assertIn("GREMLINS_FLAGS='--tags", proc.stderr)
                self.assertIn("build tags (none)", proc.stderr)

    def test_ceiling_caps_the_coefficient_and_says_so(self):
        # A floor of 1 keeps the ceilings small enough for a stub that
        # sleeps half a second: 3 s leaves a multiple under 8 for any base
        # between 0.375 s and 1.5 s, and 10 s one of at least 8 up to 1.25 s.
        for ceiling, under_the_floor in (("3", True), ("10", False)):
            with self.subTest(ceiling=ceiling):
                proc, calls = self.run_script("./internal/pkg", "30", "1", ceiling,
                                              env={"STUB_TEST_SLEEP": "0.5"})
                self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
                base = self.printed_base(proc)
                coefficient = self.coefficient(proc, calls)
                self.assertEqual(coefficient, int(int(ceiling) / base))
                self.assertLess(coefficient, int(30 / base) + 1)
                self.assertIn("%ss ceiling holds the coefficient at %d" % (ceiling, coefficient), proc.stdout)
                self.assertEqual("under the floor of 8" in proc.stdout, under_the_floor, proc.stdout)
                self.assertEqual(coefficient < 8, under_the_floor)
                self.assertLessEqual(self.printed_deadline(proc), int(ceiling))
                self.assertIn("ceiling %ss)" % ceiling, proc.stdout)

    def test_ceiling_below_the_floor_is_raised_to_it_out_loud(self):
        proc, calls = self.run_script("./internal/pkg", "30", "10", "5", env={"STUB_TEST_SLEEP": str(SLEEP)})
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assertIn("MUTANT_DEADLINE_MAX=5s is under the 10s floor", proc.stdout)
        self.assertIn("ceiling 10s)", proc.stdout)
        base = self.printed_base(proc)
        self.assertEqual(self.coefficient(proc, calls), expected_coefficient(30, base, 10))

    def test_ceiling_defaults_to_an_hour_when_the_caller_passes_three_arguments(self):
        for args, budget in ((("./internal/pkg", "30", "10"), 30), (("./internal/pkg",), DEFAULT_BUDGET)):
            with self.subTest(args=args):
                proc, calls = self.run_script(*args)
                self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
                self.assertIn("(budget %ds, ceiling %ds)" % (budget, DEFAULT_CEILING), proc.stdout)
                self.assertEqual(self.coefficient(proc, calls),
                                 expected_coefficient(budget, self.printed_base(proc), DEFAULT_CEILING))

    def test_baseline_longer_than_half_the_ceiling_is_refused(self):
        proc, calls = self.run_script("./internal/pkg", "30", "1", "1", env={"STUB_TEST_SLEEP": "0.6"})
        self.assert_refused_before_gremlins(proc, calls)
        self.assertIn("MUTANT_DEADLINE_MAX", proc.stderr)

    def test_baseline_runs_the_command_gremlins_multiplies(self):
        # gremlins' coverage step, from the module root: go test [-tags T]
        # [-coverpkg P] -cover -coverprofile F <scan>, where scan is the
        # package's subtree, or the whole module under --integration.
        pkg = "./internal/pkg/..."
        cases = [
            ("--tags e2e", {}, ["-tags", "e2e"], pkg),
            ("", {}, [], pkg),
            ("--coverpkg ./internal/...", {}, ["-coverpkg", "./internal/..."], pkg),
            ("", {"GREMLINS_UNLEASH_COVERPKG": "./..."}, ["-coverpkg", "./..."], pkg),
            ("-i", {}, [], "./..."),
            ("-di", {}, [], "./..."),
            ("--integration", {}, [], "./..."),
            ("--integration=true", {}, [], "./..."),
            ("", {"GREMLINS_UNLEASH_INTEGRATION": "true"}, [], "./..."),
            ("--integration=false", {"GREMLINS_UNLEASH_INTEGRATION": "true"}, [], pkg),
        ]
        for flags, env, extra, scan in cases:
            with self.subTest(flags=flags, env=env):
                proc, calls = self.run_script("./internal/pkg", flags=flags, env=env)
                self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
                sequence = [c["argv"][0] for c in calls if c["argv"][:1] != ["list"]]
                # Downloads first and one untimed run, so the timed one finds
                # the cache as warm as gremlins' own run will.
                self.assertEqual(sequence, ["mod", "test", "test", "run"])
                mod = self.of(calls, "mod", "download")[0]
                self.assertEqual(mod["cwd"], self.root)
                tests = self.of(calls, "test")
                for call in tests:
                    argv = call["argv"]
                    profile = argv[argv.index("-coverprofile") + 1]
                    self.assertEqual(argv, ["test", "-count=1"] + extra + [
                        "-cover", "-coverprofile", profile, scan])
                    self.assertEqual(call["cwd"], self.root)
                    self.assertFalse(os.path.exists(profile), "the baseline's profile was left behind")
                    if call["stdout"] is not None:
                        self.assertFalse(os.path.exists(call["stdout"]), "the baseline's output was left behind")
                self.assertEqual(tests[0]["argv"], tests[1]["argv"])
        # gremlins scans ./... when it is pointed at the module root itself.
        with self.subTest("the module root"):
            proc, calls = self.run_script(".")
            self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
            tests = self.of(calls, "test")
            self.assertEqual(len(tests), 2)
            for call in tests:
                self.assertEqual(call["argv"][-1], "./...")

    def test_staged_package_main_baseline_carries_the_tags_and_is_removed(self):
        staged = "./cmd/tool.mutants-main"
        proc, calls = self.run_script("./cmd/tool", flags="--tags e2e", env={
            "STUB_PKG_NAME": "main", "STUB_NEEDS_TAG": "e2e", "STUB_TEST_SLEEP": str(SLEEP)})
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        tests = self.of(calls, "test")
        self.assertEqual(len(tests), 2)
        for call in tests:
            self.assertEqual(call["argv"][-1], staged + "/...")
            self.assertEqual(self.tags_of(call["argv"]), "e2e")
            self.assertTrue(call["pattern_dir_exists"], "the baseline ran before the copy was staged")
        self.assertEqual(self.gremlins(proc, calls)["argv"][-1], staged)
        self.assertFalse(os.path.exists(os.path.join(self.root, "cmd", "tool.mutants-main")))
        self.assertGreaterEqual(self.printed_base(proc), SLEEP)

    def test_failing_baseline_is_refused(self):
        cases = [
            ("the first run fails", "1", "does not pass its own tests"),
            # A suite that passes once and fails the next time decides
            # nothing: a mutant's verdict would be a coin toss.
            ("the timed run fails", "2", "failed them on the timed second run"),
        ]
        for name, run, says in cases:
            with self.subTest(name):
                proc, calls = self.run_script("./internal/pkg", env={"STUB_TEST_FAIL_RUN": run})
                self.assert_refused_before_gremlins(proc, calls)
                self.assertIn(says, proc.stderr)
                self.assertIn("--- FAIL: TestPlanted", proc.stderr)

    def test_gremlins_runs_under_count_1_with_invert_logical_and_the_default_exclusion(self):
        cases = [
            ("", {}, True),
            ("-E x", {}, False),
            ("--exclude-files=x", {}, False),
            ("--exclude-files x", {}, False),
            ("-dE x", {}, False),
            ("", {"GREMLINS_UNLEASH_EXCLUDE_FILES": "x"}, False),
        ]
        for flags, env, default in cases:
            with self.subTest(flags=flags, env=env):
                proc, calls = self.run_script("./internal/pkg", flags=flags, env=env)
                self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
                run = self.gremlins(proc, calls)
                self.assertIn("-count=1", run["goflags"].split())
                argv = run["argv"]
                self.assertEqual(argv[2:6], ["unleash", "--invert-logical", "--workers", "4"])
                self.assertEqual(argv[-1], "./internal/pkg")
                self.assertEqual("--exclude-files=/" in argv, default, argv)
                if flags:
                    self.assertEqual(argv[8:-1][:len(flags.split())], flags.split())

    def test_flags_the_script_cannot_read_are_refused(self):
        # A flag this reading does not know could be hiding a -t whose value
        # the baseline would then not see.
        for flags, says in (("-x", "-x"), ("--tags", "--tags"), ("-dt", "-dt")):
            with self.subTest(flags=flags):
                proc, calls = self.run_script("./internal/pkg", flags=flags)
                self.assert_refused_before_gremlins(proc, calls)
                self.assertEqual(self.of(calls, "test"), [])
                self.assertIn("cannot read %s" % says, proc.stderr)

    def test_comma_decimal_locale_leaves_the_reading_intact(self):
        # bash formats `time` with the locale's decimal separator, and awk
        # reads 0,940 as 0, which would turn every bound into infinity.
        found = comma_decimal_locale(self.scratch)
        if found is None:
            self.skipTest("no comma-decimal locale is installed and none could be compiled")
        name, locpath = found
        env = {"LC_ALL": name, "LANG": name, "STUB_TEST_SLEEP": str(SLEEP)}
        if locpath:
            env["LOCPATH"] = locpath
        proc, calls = self.run_script("./internal/pkg", env=env)
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        base = self.printed_base(proc)
        self.assertGreaterEqual(base, SLEEP)
        self.assertEqual(self.coefficient(proc, calls),
                         expected_coefficient(DEFAULT_BUDGET, base, DEFAULT_CEILING))

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
