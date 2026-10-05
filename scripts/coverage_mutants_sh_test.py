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
PATH, which answers the `go env`, `go list`, `go mod download` and `go test`
calls from what each case configures, records every call, and records the
gremlins run instead of starting it. What is asserted is what reaches
gremlins: the tags every go command before it carries, the command the
baseline times, the per-mutant command run once to warm the cache, the
-trimpath and GOROOT every command inherits (issue 1029), the coefficient
derived from the printed base or the one the caller fixed, the ceiling on the
deadline, and the refusals that stop a run whose figures would mean nothing.
For a package gremlins cannot resolve where it is (issue 872), it is also
where the copy the script measures instead is staged, beside the package or,
when the package imports its own internal/ subtree, below it, and what that
copy holds: the package and its fixtures, and none of the packages below it.

Run with:

    python3 -m unittest discover -s scripts -p 'coverage_mutants_sh_test.py'
"""

import json
import os
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import time
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
SCRIPT = os.path.join(ROOT, "scripts", "coverage-mutants.sh")

# Long enough that a baseline which really ran the stand-in's `go test` is
# told apart from one that did not, and short enough that the whole file
# stays in seconds: every successful case runs the baseline twice.
SLEEP = 0.3

# The script's own defaults. `make coverage-mutants` always passes all three,
# as MUTANT_BUDGET, MUTANT_BUDGET_FLOOR and MUTANT_DEADLINE_MAX, so the
# Makefile's are the ones a run gets, and
# test_makefile_passes_the_scripts_defaults holds the two sets together.
DEFAULT_BUDGET = 300
DEFAULT_FLOOR = 10
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


def import_path(directory):
    below = os.path.relpath(directory, env["STUB_ROOT"])
    return "example.com/m" if below == "." else "example.com/m/" + below.replace(os.sep, "/")


def import_paths(pattern):
    """The packages a pattern names, as go list resolves it from the working
    directory: a directory, or one ending in /... that takes every package
    below it. A pattern matching nothing names nothing."""
    recursive = pattern.endswith("/...")
    base = os.path.normpath(os.path.join(os.getcwd(), pattern[:-4] if recursive else pattern))
    found = []
    for directory, _, files in os.walk(base):
        if any(name.endswith(".go") for name in files):
            found.append(import_path(directory))
        if not recursive:
            break
    return found


def package_dirs(base):
    """The directories `go list ./...` names from base: every one holding a .go
    file, and never one go ignores, testdata or a name beginning with . or _,
    nor anything below such a one."""
    found = []
    for directory, subdirs, files in os.walk(base):
        subdirs[:] = sorted(d for d in subdirs if d != "testdata" and d[0] not in "._")
        if any(name.endswith(".go") for name in files):
            found.append(directory)
    return found


def shown(directory):
    """A directory as go list prints it. Under STUB_DIR_SEPARATOR, the part
    below the module root as Windows prints it: the root is left as it is,
    since the script changes into it."""
    separator = env.get("STUB_DIR_SEPARATOR")
    if not separator or directory == env["STUB_ROOT"]:
        return directory
    below = os.path.relpath(directory, env["STUB_ROOT"])
    return env["STUB_ROOT"] + separator + below.replace("/", separator)


def tree(directory):
    """Every file below a directory, by its path relative to it."""
    files = []
    for parent, _, names in os.walk(directory):
        for name in names:
            files.append(os.path.relpath(os.path.join(parent, name), directory))
    return sorted(files)


entry = {"argv": args, "cwd": os.getcwd(), "goflags": env.get("GOFLAGS", ""), "goroot": env.get("GOROOT")}
try:
    entry["stdout"] = os.readlink("/proc/self/fd/1")
except OSError:
    entry["stdout"] = None

status = 0
if args == ["env", "GOROOT"]:
    print(env.get("STUB_GOROOT", "/stub/goroot"))
elif args[:2] == ["list", "-m"]:
    print(render(args[args.index("-f") + 1], {"{{.Dir}}": env["STUB_ROOT"]}))
elif args[:1] == ["list"] and args[args.index("-f") + 1] == "{{.ImportPath}}":
    # The -coverpkg resolution: every pattern after the template. An empty
    # one is refused the way go refuses it, with a status and no listing.
    patterns = args[args.index("-f") + 2:]
    if "" in patterns:
        sys.stderr.write('go: invalid package: ""\n')
        status = 1
    else:
        for pattern in patterns:
            for listed in import_paths(pattern):
                print(listed)
elif args[:1] == ["list"] and args[args.index("-f") + 1] == "{{.Dir}}":
    # The packages below a staged package, listed from its own directory.
    if args[-1] != "./...":
        sys.stderr.write("stub go list names packages below the working directory only, not %r\n" % args[-1])
        status = 2
    else:
        for directory in package_dirs(os.getcwd()):
            print(shown(directory))
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
            "{{.Dir}}": shown(pkgdir),
            "{{.ImportPath}}": import_path(pkgdir),
            "{{len .TestGoFiles}}": tests,
            "{{len .XTestGoFiles}}": xtests,
            '{{join .Imports " "}}': env.get("STUB_IMPORTS", ""),
            '{{join .TestImports " "}}': env.get("STUB_TEST_IMPORTS", ""),
            '{{join .XTestImports " "}}': env.get("STUB_XTEST_IMPORTS", ""),
            "{{with .Error}}{{.}}{{end}}": error,
        }))
elif args[:2] == ["mod", "download"]:
    pass
elif args[:1] == ["test"]:
    pattern = args[-1]
    pattern_dir = os.path.join(os.getcwd(), pattern.rstrip(".").rstrip("/"))
    entry["pattern_dir_exists"] = os.path.isdir(pattern_dir)
    # What the run found there, which for a staged copy is what was copied.
    entry["tree"] = tree(pattern_dir) if entry["pattern_dir_exists"] else None
    if not tagged(args):
        print("?   \t%s\t[no test files]" % pattern)
    else:
        with open(env["STUB_LOG"], encoding="utf-8") as fh:
            run = 1 + sum(1 for line in fh if json.loads(line)["argv"][:1] == ["test"])
        if env.get("STUB_STARTED_ON") == "test":
            open(env["STUB_STARTED"], "w", encoding="utf-8").close()
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
    entry["target_exists"] = os.path.isdir(os.path.join(os.getcwd(), args[-1]))
    if env.get("STUB_STARTED_ON") == "run":
        open(env["STUB_STARTED"], "w", encoding="utf-8").close()
    time.sleep(float(env.get("STUB_GREMLINS_SLEEP", "0")))
else:
    sys.stderr.write("stub go: unexpected call %r\n" % (args,))
    status = 2

entry["status"] = status
with open(env["STUB_LOG"], "a", encoding="utf-8") as fh:
    fh.write(json.dumps(entry) + "\n")
sys.exit(status)
'''


def file_tree(directory):
    """Every file below a directory, by its path relative to it, the way the
    stand-in records what a run found."""
    files = []
    for parent, _, names in os.walk(directory):
        for name in names:
            files.append(os.path.relpath(os.path.join(parent, name), directory))
    return sorted(files)


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

    @staticmethod
    def baselines(calls):
        """The coverage runs: the untimed gate and the timed one."""
        return [c for c in calls if c["argv"][:1] == ["test"] and "-coverprofile" in c["argv"]]

    @staticmethod
    def warmups(calls):
        """The runs of the command gremlins runs against each mutant."""
        return [c for c in calls if c["argv"][:1] == ["test"] and "-coverprofile" not in c["argv"]]

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

    # The import path of the stand-in command's own internal subtree, and the
    # two places a copy of the command can be staged.
    OWN_INTERNAL = "example.com/m/cmd/tool/internal"
    BESIDE = "cmd/tool.mutants-main"
    BELOW = "cmd/tool/tool.mutants-main"

    def write(self, path, text=""):
        full = os.path.join(self.root, path)
        os.makedirs(os.path.dirname(full), exist_ok=True)
        with open(full, "w", encoding="utf-8") as fh:
            fh.write(text)

    def give_the_command_a_subtree(self):
        """Gives cmd/tool what a command such as cmd/gen_action_grants holds
        below it: an internal/ tree of packages with a file beside them, a
        package further down a directory that is not one, fixtures, and the
        directories go never reads as packages. Returns the files a staged copy
        of it should hold: everything but the packages below it."""
        self.write("cmd/tool/tool_test.go", "package main\n")
        self.write("cmd/tool/.keep")
        self.write("cmd/tool/testdata/fixture.txt")
        self.write("cmd/tool/testdata/planted/planted.go", "package planted\n")
        self.write("cmd/tool/assets/bg.jpg")
        self.write("cmd/tool/assets/gen/gen.go", "package gen\n")
        self.write("cmd/tool/_scratch/scratch.go", "package scratch\n")
        self.write("cmd/tool/internal/NOTES.md")
        self.write("cmd/tool/internal/derive/derive.go", "package derive\n")
        self.write("cmd/tool/internal/derive/testdata/derive.txt")
        self.write("cmd/tool/internal/join/join.go", "package join\n")
        return [".keep", "_scratch/scratch.go", "assets/bg.jpg", "doc.go", "internal/NOTES.md",
                "testdata/fixture.txt", "testdata/planted/planted.go", "tool_test.go"]

    def test_baseline_without_a_duration_on_its_last_line_is_timed_by_the_clock(self):
        # A -cover run ends in a coverage figure, so the last field of its
        # last line is "statements" and carries no duration at all. The other
        # two lines carry a duration go test would report, far above and far
        # below the clock's, so a script that read either figure off the
        # summary line, or the larger of the two, prints a base outside
        # [SLEEP, 42).
        cases = [
            ("no duration", None),
            ("a duration far above the clock's",
             "ok  \t./internal/pkg/...\t42.000s\tcoverage: 71.4% of statements"),
            ("a duration far below the clock's",
             "ok  \t./internal/pkg/...\t0.001s\tcoverage: 71.4% of statements"),
        ]
        for name, last_line in cases:
            with self.subTest(name):
                env = {"STUB_TEST_SLEEP": str(SLEEP)}
                if last_line is not None:
                    env["STUB_TEST_LAST_LINE"] = last_line
                proc, calls = self.run_script("./internal/pkg", env=env)
                self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
                base = self.printed_base(proc)
                self.assertGreaterEqual(base, SLEEP)
                self.assertLess(base, 42)
                coefficient = self.coefficient(proc, calls)
                self.assertEqual(coefficient, expected_coefficient(DEFAULT_BUDGET, base, DEFAULT_CEILING))
                # The budget is what each mutant gets, give or take one
                # multiple of the base, rather than the multiple of it a
                # misread base used to hand gremlins: about 9 times the budget
                # on elicitationtools, and about 11,400 times on the e2e
                # harness.
                deadline = self.printed_deadline(proc)
                self.assertAlmostEqual(deadline, base * coefficient, delta=0.1)
                self.assertGreaterEqual(deadline, DEFAULT_BUDGET)
                self.assertLessEqual(deadline, DEFAULT_BUDGET + base + 0.1)

    def test_coefficient_is_never_under_eight(self):
        # A budget of 2 s over a base of at least 0.25 s is a multiple of 8 or
        # less, so the floor of 8 is what gives the package a real multiple
        # of its own runtime, and the deadline lands above the budget.
        proc, calls = self.run_script("./internal/pkg", "2", "1", env={"STUB_TEST_SLEEP": str(SLEEP)})
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        base = self.printed_base(proc)
        self.assertGreaterEqual(base, 0.25)
        coefficient = self.coefficient(proc, calls)
        self.assertEqual(coefficient, 8)
        self.assertEqual(coefficient, expected_coefficient(2, base, DEFAULT_CEILING))
        deadline = self.printed_deadline(proc)
        self.assertAlmostEqual(deadline, 8 * base, delta=0.1)
        self.assertGreater(deadline, 2)

    def test_coefficient_is_never_over_six_thousand(self):
        # A 6000 s budget over a base under a second asks for a multiple past
        # 6000, and a ceiling of 100000 s leaves room for all of it, so only
        # the coefficient's own clamp holds it, and no ceiling is reported
        # as having done so.
        proc, calls = self.run_script("./internal/pkg", "6000", "1", "100000",
                                      env={"STUB_TEST_SLEEP": str(SLEEP)})
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        base = self.printed_base(proc)
        self.assertLess(base, 1)
        self.assertEqual(self.coefficient(proc, calls), 6000)
        self.assertEqual(expected_coefficient(6000, base, 100000), 6000)
        self.assertNotIn("ceiling holds", proc.stdout)

    def test_ceiling_past_what_awk_prints_as_an_integer_is_still_compared(self):
        # Older mawk builds (1.3.4 20200120, the awk Debian 12 installs) print
        # an integer past 2^31 in exponent form, so the ceiling's multiple has
        # to be clamped before bash compares it: a ceiling of 1e9 s over a
        # base of about 0.3 s is a multiple of about 3e9. The case tells the
        # clamp from its absence only where the awk on PATH is such a build;
        # gawk, BWK awk, busybox awk and current mawk print the integer.
        proc, calls = self.run_script("./internal/pkg", "30", "1", "1000000000",
                                      env={"STUB_TEST_SLEEP": str(SLEEP)})
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assertNotIn("integer expression expected", proc.stderr)
        self.assertEqual(self.coefficient(proc, calls),
                         expected_coefficient(30, self.printed_base(proc), 1000000000))

    def test_knobs_that_are_not_seconds_are_refused(self):
        # awk reads the leading digits of anything and drops the rest, so a
        # ceiling written 2h would have been read as 2 and raised to the
        # floor, handing every mutant about ten seconds.
        cases = [
            (("./internal/pkg", "30", "10", "2h"), "MUTANT_DEADLINE_MAX=2h"),
            (("./internal/pkg", "5m"), "MUTANT_BUDGET=5m"),
            (("./internal/pkg", "30", "ten"), "MUTANT_BUDGET_FLOOR=ten"),
            (("./internal/pkg", "30", "10", "-1"), "MUTANT_DEADLINE_MAX=-1"),
            (("./internal/pkg", "30", "10", "1e9"), "MUTANT_DEADLINE_MAX=1e9"),
        ]
        for args, says in cases:
            with self.subTest(args=args):
                proc, calls = self.run_script(*args)
                self.assert_refused_before_gremlins(proc, calls)
                self.assertEqual(calls, [], "a go command ran before the knobs were read")
                self.assertIn(says + " is not a number of seconds", proc.stderr)

    def test_makefile_passes_the_scripts_defaults(self):
        # make always passes the three knobs, so the script's own defaults
        # never apply through it: the 300 s budget that measured first-mutant
        # recompiles as kills rather than timeouts holds only as long as the
        # Makefile says 300 too.
        with open(os.path.join(ROOT, "Makefile"), encoding="utf-8") as fh:
            makefile = fh.read()
        for name, want in (("MUTANT_BUDGET", DEFAULT_BUDGET), ("MUTANT_BUDGET_FLOOR", DEFAULT_FLOOR),
                           ("MUTANT_DEADLINE_MAX", DEFAULT_CEILING)):
            with self.subTest(name):
                match = re.search(r"^%s \?= (\S+)$" % name, makefile, re.MULTILINE)
                self.assertIsNotNone(match, "no `%s ?=` default in the Makefile" % name)
                self.assertEqual(match.group(1), str(want))
        with self.subTest("the recipe passes all four, in order"):
            self.assertRegex(
                makefile,
                r"(?m)^\t@scripts/coverage-mutants\.sh \$\(PKG\) \$\(MUTANT_BUDGET\) "
                r"\$\(MUTANT_BUDGET_FLOOR\) \$\(MUTANT_DEADLINE_MAX\)$")

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
            # pflag stops at `--`, so what follows it is no flag of gremlins'
            # and must not reach the baseline as one.
            ("--tags e2e -- -tother", {}),
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
            ("tagged tests, untagged run", "", {"STUB_NEEDS_TAG": "e2e"}, "no test files"),
            # No tests at all.
            ("no tests", "", {"STUB_TEST_FILES": "0 0"}, "no test files"),
            # One of the two that would let tests elsewhere measure it is not
            # enough: without --integration each mutant runs only this
            # package's tests, and without -coverpkg nothing covers it.
            ("no tests, integration alone", "-i", {"STUB_TEST_FILES": "0 0"}, "no test files"),
            ("no tests, -coverpkg alone", "--coverpkg ./...", {"STUB_TEST_FILES": "0 0"}, "no test files"),
            # Every file behind the tag: go list itself reports the package
            # unloadable, which must reach the reader as the same refusal.
            ("every file tagged", "", {"STUB_NEEDS_TAG": "e2e", "STUB_ALL_TAGGED": "1"},
             "build constraints exclude all Go files"),
        ]
        for name, flags, env, says in cases:
            with self.subTest(name):
                proc, calls = self.run_script("./internal/pkg", flags=flags, env=env)
                self.assert_refused_before_gremlins(proc, calls)
                self.assertEqual(self.of(calls, "test"), [])
                self.assertIn(says, proc.stderr)
                self.assertIn("GREMLINS_FLAGS='--tags", proc.stderr)
                self.assertIn("GREMLINS_FLAGS='-i --coverpkg", proc.stderr)
                self.assertIn("build tags (none)", proc.stderr)

    def test_package_whose_tests_are_elsewhere_is_measured_under_integration_and_coverpkg(self):
        # Under both, the module's other tests cover the package's blocks and
        # gremlins runs them against each mutant, so a mutant can be killed
        # and the run is measured rather than refused. The -coverpkg names the
        # package in each spelling go test reads one in: a pattern enclosing
        # it, the package itself, and a comma-separated list whose second
        # entry is the one naming it.
        for flags in ("-i --coverpkg ./...", "--integration --coverpkg=./internal/...",
                      "-i --coverpkg ./internal/pkg", "-i --coverpkg ./cmd/...,./internal/pkg"):
            with self.subTest(flags=flags):
                proc, calls = self.run_script("./internal/pkg", flags=flags, env={"STUB_TEST_FILES": "0 0"})
                self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
                self.assertNotIn("refusing", proc.stderr)
                self.assertIn("measuring it through the module's other tests", proc.stdout)
                self.gremlins(proc, calls)
                for call in self.of(calls, "test"):
                    self.assertEqual(call["argv"][-1], "./...")
        with self.subTest("the pattern is resolved from the root under the tags gremlins is given"):
            proc, calls = self.run_script("./internal/pkg", flags="--tags e2e -i --coverpkg ./internal/...",
                                          env={"STUB_TEST_FILES": "0 0"})
            self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
            resolutions = [c for c in self.package_lists(calls) if "{{.ImportPath}}" in c["argv"]]
            self.assertEqual(len(resolutions), 1, calls)
            self.assertEqual(resolutions[0]["cwd"], self.root)
            self.assertEqual(self.tags_of(resolutions[0]["argv"]), "e2e")
            self.assertEqual(resolutions[0]["argv"][-1], "./internal/...")

    def test_coverpkg_that_does_not_name_the_package_is_refused(self):
        # A -coverpkg that names only other packages leaves this one's blocks
        # exactly as uncovered as none would, so every mutant would be NOT
        # COVERED: the outcome the refusal exists to prevent, which the run
        # used to proceed into while saying the -coverpkg let it be covered.
        for flags in ("-i --coverpkg ./cmd/...", "-i --coverpkg ./internal/other/...",
                      "--integration --coverpkg=./cmd/tool,./internal/pkg/sub"):
            with self.subTest(flags=flags):
                proc, calls = self.run_script("./internal/pkg", flags=flags, env={"STUB_TEST_FILES": "0 0"})
                self.assert_refused_before_gremlins(proc, calls)
                self.assertEqual(self.of(calls, "test"), [])
                self.assertIn("does not name example.com/m/internal/pkg", proc.stderr)
                self.assertNotIn("measuring it through the module's other tests", proc.stdout)
                self.assertIn("GREMLINS_FLAGS='-i --coverpkg <pattern>', a pattern that names it", proc.stderr)

    def test_package_no_other_test_can_link_is_refused_whatever_coverpkg_says(self):
        # A package main, and a package measured through a staged copy, are
        # linked by no other package's test, and gremlins matches the staged
        # copy's files by their own path, so no -coverpkg covers either and
        # every mutant would be reported NOT COVERED. The run used to say it
        # was measuring such a package through the module's other tests and
        # hand gremlins the staged copy.
        cases = [
            ("a package main", "./cmd/tool", {"STUB_PKG_NAME": "main"}),
            ("a package staged for its directory's name", "./internal/pkg", {"STUB_PKG_NAME": "other"}),
        ]
        for name, pkg, env in cases:
            for flags in ("-i --coverpkg " + pkg, "-i --coverpkg ./..."):
                with self.subTest(name, flags=flags):
                    proc, calls = self.run_script(pkg, flags=flags, env=dict(env, STUB_TEST_FILES="0 0"))
                    self.assert_refused_before_gremlins(proc, calls)
                    self.assertEqual(self.of(calls, "test"), [])
                    self.assertIn("no other package's test can link", proc.stderr)
                    self.assertNotIn("measuring it through the module's other tests", proc.stdout)
                    self.assertNotIn("GREMLINS_FLAGS='-i --coverpkg", proc.stderr)
                    self.assertIn("GREMLINS_FLAGS='--tags", proc.stderr)
                    self.assertFalse(os.path.exists(os.path.join(self.root, pkg[2:] + ".mutants-" + env["STUB_PKG_NAME"])))

    def test_coverpkg_go_cannot_resolve_is_refused_as_such(self):
        # A -coverpkg with an empty entry is one go list refuses outright,
        # which is not the same answer as a pattern that resolved and missed
        # the package, and the refusal says which it was.
        proc, calls = self.run_script("./internal/pkg", flags="-i --coverpkg ./internal/pkg,,./cmd/tool",
                                      env={"STUB_TEST_FILES": "0 0"})
        self.assert_refused_before_gremlins(proc, calls)
        self.assertEqual(self.of(calls, "test"), [])
        self.assertIn("go list could not resolve -coverpkg ./internal/pkg,,./cmd/tool", proc.stderr)
        self.assertIn('go: invalid package: ""', proc.stderr)
        self.assertNotIn("does not name", proc.stderr)

    def test_package_with_test_files_of_either_kind_is_measured(self):
        # internal/cachehints is the shape of the second case: its one test
        # file declares package cachehints_test.
        for files in ("0 2", "2 0"):
            with self.subTest(files=files):
                proc, calls = self.run_script("./internal/pkg", env={"STUB_TEST_FILES": files})
                self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
                self.assertNotIn("no test files", proc.stdout + proc.stderr)
                self.gremlins(proc, calls)

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

    def test_a_coefficient_the_caller_fixes_is_the_one_announced_and_passed(self):
        # gremlins reads a --timeout-coefficient in GREMLINS_FLAGS in place of
        # the derived one, so announcing the derived deadline printed one that
        # was not in force, in exactly the mode the sweeps and issue 1029 ran
        # in. The script hands gremlins only the caller's, and announces it as
        # gremlins reads it: 0 is gremlins' own default of 3. The budget and
        # the ceiling bound only a derived coefficient, so a ceiling that could
        # not hold two runs refuses nothing here.
        cases = [
            ("--workers 2 --timeout-coefficient 20", (), 20),
            ("--timeout-coefficient=20", (), 20),
            ("--timeout_coefficient 20", (), 20),
            ("--timeout-coefficient 5 --timeout-coefficient 40", (), 40),
            ("--timeout-coefficient 0", (), 3),
            ("--timeout-coefficient 20", ("30", "1", "1"), 20),
        ]
        for flags, knobs, applied in cases:
            with self.subTest(flags=flags, knobs=knobs):
                proc, calls = self.run_script("./internal/pkg", *knobs, flags=flags,
                                              env={"STUB_TEST_SLEEP": "0.6" if knobs else str(SLEEP)})
                self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
                base = self.printed_base(proc)
                self.assertIn("GREMLINS_FLAGS fixes -timeout-coefficient %d: about" % applied, proc.stdout)
                self.assertAlmostEqual(self.printed_deadline(proc), base * applied, delta=0.06)
                self.assertIn("which neither the", proc.stdout)
                self.assertNotIn("so -timeout-coefficient", proc.stdout)
                self.assertNotIn("ceiling holds the coefficient", proc.stdout)
                # The caller's words follow the script's four directly: no
                # derived coefficient is passed beside theirs.
                argv = self.gremlins(proc, calls)["argv"]
                self.assertEqual(argv[2:6], ["unleash", "--invert-logical", "--workers", "4"])
                self.assertEqual(argv[6:-1], flags.split() + ["--exclude-files=/"])
                self.assertEqual(argv[-1], "./internal/pkg")
        # A value the script cannot read the way pflag and gremlins do is
        # refused before the suite runs, rather than announced wrong.
        for flags in ("--timeout-coefficient 020", "--timeout-coefficient x", "--timeout-coefficient=",
                      "--timeout-coefficient -3"):
            with self.subTest(flags=flags):
                proc, calls = self.run_script("./internal/pkg", flags=flags)
                self.assert_refused_before_gremlins(proc, calls)
                self.assertEqual(self.of(calls, "test"), [])
                self.assertIn("is not a whole number written without a leading zero", proc.stderr)

    def test_a_callers_own_exclusion_warms_every_package_below_the_target(self):
        # The default exclusion keeps every mutant in the target, whose own
        # `go test <package>` is what the warm-up runs. A caller who states
        # their own --exclude-files may leave packages below it to be mutated,
        # each run as its own `go test <subpackage>`, whose plain build the
        # coverage runs never compile, so the warm-up covers the subtree.
        cases = [
            ("", {}, "./internal/pkg"),
            ("-E x", {}, "./internal/pkg/..."),
            ("--exclude-files=x", {}, "./internal/pkg/..."),
            ("", {"GREMLINS_UNLEASH_EXCLUDE_FILES": "x"}, "./internal/pkg/..."),
            ("-i -E x", {}, "./..."),
        ]
        for flags, env, each in cases:
            with self.subTest(flags=flags, env=env):
                proc, calls = self.run_script("./internal/pkg", flags=flags, env=env)
                self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
                self.assertEqual([c["argv"][-1] for c in self.warmups(calls)], [each])
        with self.subTest("a staged copy"):
            proc, calls = self.run_script("./cmd/tool", flags="-E x", env={"STUB_PKG_NAME": "main"})
            self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
            warmups = self.warmups(calls)
            self.assertEqual([c["argv"][-1] for c in warmups], ["./cmd/tool.mutants-main/..."])
            self.assertTrue(warmups[0]["pattern_dir_exists"])

    def test_baseline_runs_the_command_gremlins_multiplies(self):
        # gremlins' coverage step, from the module root: go test [-tags T]
        # [-coverpkg P] -cover -coverprofile F <scan>, where scan is the
        # package's subtree, or the whole module under --integration. Between
        # the gate and the timed run, the command gremlins runs against each
        # mutant: go test [-tags T] -failfast <each>, where each is the package
        # itself, or the whole module under --integration, and never carries
        # the -coverpkg, which gremlins passes only to its coverage step.
        pkg = "./internal/pkg/..."
        tags = ["-tags", "e2e"]
        cases = [
            ("--tags e2e", {}, tags, [], pkg),
            ("", {}, [], [], pkg),
            ("--coverpkg ./internal/...", {}, [], ["-coverpkg", "./internal/..."], pkg),
            ("", {"GREMLINS_UNLEASH_COVERPKG": "./..."}, [], ["-coverpkg", "./..."], pkg),
            ("--tags e2e --coverpkg ./internal/...", {}, tags, ["-coverpkg", "./internal/..."], pkg),
            ("-i", {}, [], [], "./..."),
            ("-di", {}, [], [], "./..."),
            ("-i=false", {}, [], [], pkg),
            ("-di=true", {}, [], [], "./..."),
            ("--integration", {}, [], [], "./..."),
            ("--integration=true", {}, [], [], "./..."),
            # gremlins v0.6.0 reads the variable back as a string and asserts
            # a bool, so it never widens gremlins' run and must not widen the
            # baseline's.
            ("", {"GREMLINS_UNLEASH_INTEGRATION": "true"}, [], [], pkg),
            ("--integration=false", {"GREMLINS_UNLEASH_INTEGRATION": "true"}, [], [], pkg),
            ("-i", {"GREMLINS_UNLEASH_INTEGRATION": "false"}, [], [], "./..."),
        ]
        for flags, env, tag_args, cover_args, scan in cases:
            with self.subTest(flags=flags, env=env):
                proc, calls = self.run_script("./internal/pkg", flags=flags, env=env)
                self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
                self.assertEqual("GREMLINS_UNLEASH_INTEGRATION is set" in proc.stdout,
                                 "GREMLINS_UNLEASH_INTEGRATION" in env, proc.stdout)
                # The notice says what decided the run: that it is not an
                # integration run only where no flag made it one.
                self.assertEqual("so this is not an integration run" in proc.stdout,
                                 "GREMLINS_UNLEASH_INTEGRATION" in env and scan != "./...", proc.stdout)
                self.assertEqual("this is an integration run because GREMLINS_FLAGS asks for one" in proc.stdout,
                                 "GREMLINS_UNLEASH_INTEGRATION" in env and scan == "./...", proc.stdout)
                sequence = [c["argv"][0] for c in calls if c["argv"][:1] != ["list"]]
                # The toolchain's root first, then downloads, the untimed gate,
                # the per-mutant command, and the timed run, which finds the
                # cache as warm as gremlins' own coverage run will.
                self.assertEqual(sequence, ["env", "mod", "test", "test", "test", "run"])
                mod = self.of(calls, "mod", "download")[0]
                self.assertEqual(mod["cwd"], self.root)
                tests = self.of(calls, "test")
                baselines = self.baselines(calls)
                self.assertEqual([tests[0], tests[2]], baselines)
                for call in baselines:
                    argv = call["argv"]
                    profile = argv[argv.index("-coverprofile") + 1]
                    self.assertEqual(argv, ["test", "-count=1"] + tag_args + cover_args + [
                        "-cover", "-coverprofile", profile, scan])
                    self.assertEqual(call["cwd"], self.root)
                    self.assertFalse(os.path.exists(profile), "the baseline's profile was left behind")
                    if call["stdout"] is not None:
                        self.assertFalse(os.path.exists(call["stdout"]), "the baseline's output was left behind")
                self.assertEqual(baselines[0]["argv"], baselines[1]["argv"])
                each = "./..." if scan == "./..." else "./internal/pkg"
                self.assertEqual([c["argv"] for c in self.warmups(calls)],
                                 [["test", "-count=1"] + tag_args + ["-failfast", each]])
                self.assertEqual(tests[1]["cwd"], self.root)
        # gremlins scans ./... when it is pointed at the module root itself,
        # and runs each mutant against the module's root package.
        with self.subTest("the module root"):
            proc, calls = self.run_script(".")
            self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
            baselines = self.baselines(calls)
            self.assertEqual(len(baselines), 2)
            for call in baselines:
                self.assertEqual(call["argv"][-1], "./...")
            self.assertEqual([c["argv"][-1] for c in self.warmups(calls)], ["."])
        # go list prints native paths, so on Windows the package directory
        # carries backslashes below the root, and the pattern must still be
        # the package's subtree written with slashes.
        with self.subTest("a package directory with backslashes"):
            proc, calls = self.run_script("./internal/pkg", env={"STUB_DIR_SEPARATOR": "\\"})
            self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
            tests = self.of(calls, "test")
            self.assertEqual(len(tests), 3)
            self.assertEqual([c["argv"][-1] for c in tests], [pkg, "./internal/pkg", pkg])
            for call in tests:
                self.assertTrue(call["pattern_dir_exists"], call["argv"])
        # A staged package on Windows: the copy is made beside a directory go
        # listed with backslashes, and gremlins is handed its module-relative
        # target rather than the native path the strip used to leave whole.
        with self.subTest("a staged package directory with backslashes"):
            proc, calls = self.run_script("./internal/pkg", env={"STUB_DIR_SEPARATOR": "\\",
                                                                  "STUB_PKG_NAME": "main"})
            self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
            tests = self.of(calls, "test")
            self.assertEqual(len(tests), 3)
            staged = "./internal/pkg.mutants-main"
            self.assertEqual([c["argv"][-1] for c in tests], [staged + "/...", staged, staged + "/..."])
            for call in tests:
                self.assertTrue(call["pattern_dir_exists"], call["argv"])
            self.assertEqual(self.gremlins(proc, calls)["argv"][-1], staged)
            self.assertFalse(os.path.exists(os.path.join(self.root, "internal", "pkg.mutants-main")))

    def test_staged_package_main_baseline_carries_the_tags_and_is_removed(self):
        staged = "./cmd/tool.mutants-main"
        proc, calls = self.run_script("./cmd/tool", flags="--tags e2e", env={
            "STUB_PKG_NAME": "main", "STUB_NEEDS_TAG": "e2e", "STUB_TEST_SLEEP": str(SLEEP)})
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        tests = self.of(calls, "test")
        self.assertEqual(len(tests), 3)
        # The copy is what gremlins runs each mutant against, so it is the
        # copy the per-mutant command warms, not the original.
        self.assertEqual([c["argv"][-1] for c in tests], [staged + "/...", staged, staged + "/..."])
        for call in tests:
            self.assertEqual(self.tags_of(call["argv"]), "e2e")
            self.assertTrue(call["pattern_dir_exists"], "the baseline ran before the copy was staged")
        self.assertEqual(self.gremlins(proc, calls)["argv"][-1], staged)
        self.assertFalse(os.path.exists(os.path.join(self.root, "cmd", "tool.mutants-main")))
        self.assertGreaterEqual(self.printed_base(proc), SLEEP)
        # The listing of the packages the copy leaves out is a go command
        # before gremlins too, and loads under the same tags.
        lists = self.package_lists(calls)
        self.assertEqual(len(lists), 2, lists)
        for call in lists:
            self.assertEqual(self.tags_of(call["argv"]), "e2e", call["argv"])

    def assert_staged_at(self, proc, calls, staged, copied, says):
        """Holds a run to having measured a copy staged at staged, the
        module-relative path, which held exactly the files copied and is gone
        afterwards, and to the announcement saying where it was put."""
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assertIn("measuring a staged copy at " + staged + says, proc.stdout)
        tests = self.of(calls, "test")
        scan = "./" + staged + "/..."
        self.assertEqual([c["argv"][-1] for c in tests], [scan, "./" + staged, scan])
        for call in tests:
            self.assertTrue(call["pattern_dir_exists"], call["argv"])
        for call in self.baselines(calls):
            self.assertEqual(call["tree"], copied)
        self.assertEqual(self.gremlins(proc, calls)["argv"][-1], "./" + staged)
        self.assertFalse(os.path.exists(os.path.join(self.root, staged)), "the staged copy was left behind")
        # The packages it leaves out are listed from the package's own
        # directory, which names its subtree.
        listed = [c for c in self.package_lists(calls) if "{{.Dir}}" in c["argv"]]
        self.assertEqual(len(listed), 1, calls)
        self.assertEqual(listed[0]["cwd"], os.path.join(self.root, "cmd", "tool"))
        self.assertEqual(listed[0]["argv"][-1], "./...")

    def test_package_importing_its_own_internal_subtree_is_staged_below_it(self):
        # cmd/gen_action_grants imports cmd/gen_action_grants/internal/..., which
        # Go lets only cmd/gen_action_grants and the packages below it import,
        # so its copy beside it did not build and the run was refused. Below it,
        # every import it makes is legal. An import from the package's files,
        # from its tests or from its external tests decides it alike, and so
        # does one of the internal package itself rather than of one below it.
        copied = self.give_the_command_a_subtree()
        own = self.OWN_INTERNAL
        cases = [
            ("its own files", {"STUB_IMPORTS": "fmt %s/derive example.com/m/internal/pkg" % own}),
            ("its test files", {"STUB_TEST_IMPORTS": "testing %s/join" % own}),
            ("its external test files", {"STUB_XTEST_IMPORTS": "%s/derive" % own}),
            ("the internal package itself", {"STUB_IMPORTS": own}),
        ]
        before = file_tree(os.path.join(self.root, "cmd", "tool"))
        for name, env in cases:
            with self.subTest(name):
                proc, calls = self.run_script("./cmd/tool", env=dict(env, STUB_PKG_NAME="main"))
                self.assert_staged_at(proc, calls, self.BELOW, copied,
                                      ", one level below it, since it imports its own internal/ subtree")
                self.assertFalse(os.path.exists(os.path.join(self.root, self.BESIDE)))
                self.assertEqual(file_tree(os.path.join(self.root, "cmd", "tool")), before,
                                 "the package itself was changed")

    def test_package_importing_no_internal_subtree_of_its_own_is_staged_beside_it(self):
        # Beside the package is where every relative path a test climbs out by
        # still lands, so it stays the place for every package it builds for,
        # which is any whose internal imports are rooted above it. An import is
        # read whole: a package whose name only begins with internal, or a path
        # that only ends in the package's own internal one, is no import of its
        # internal subtree. The copy leaves out the packages below it here too.
        copied = self.give_the_command_a_subtree()
        cases = [
            ("no imports at all", {}),
            ("an internal subtree above it",
             {"STUB_IMPORTS": "example.com/m/cmd/internal/shared example.com/m/internal/pkg"}),
            ("a package whose name only begins with internal",
             {"STUB_IMPORTS": "example.com/m/cmd/tool/internalx"}),
            ("a path that only ends in its own internal one",
             {"STUB_TEST_IMPORTS": "example.org/fork/example.com/m/cmd/tool/internal/derive"}),
        ]
        for name, env in cases:
            with self.subTest(name):
                proc, calls = self.run_script("./cmd/tool", env=dict(env, STUB_PKG_NAME="main"))
                self.assert_staged_at(proc, calls, self.BESIDE, copied, "\n")
                self.assertNotIn("below it", proc.stdout)
                self.assertFalse(os.path.exists(os.path.join(self.root, self.BELOW)))

    def test_staged_copy_of_a_windows_listing_still_leaves_the_packages_below_it_out(self):
        # go list prints native paths, and the packages below the command are
        # matched against paths the script writes with slashes, so a listing
        # with backslashes has to be normalized or the copy takes internal/
        # whole.
        copied = self.give_the_command_a_subtree()
        proc, calls = self.run_script("./cmd/tool", env={
            "STUB_PKG_NAME": "main", "STUB_DIR_SEPARATOR": "\\", "STUB_IMPORTS": self.OWN_INTERNAL + "/derive"})
        self.assert_staged_at(proc, calls, self.BELOW, copied, ", one level below it")

    def test_a_leftover_copy_in_either_place_is_refused_and_kept(self):
        # A run killed hard enough to skip its trap leaves its copy behind, a
        # second package in the module, and the next run refuses rather than
        # removing what it did not create. Either place counts whichever this
        # run would use, so a package whose imports moved it is told too.
        cases = [
            ("beside it, staging beside it", self.BESIDE, {}),
            ("beside it, staging below it", self.BESIDE, {"STUB_IMPORTS": self.OWN_INTERNAL + "/derive"}),
            ("below it, staging beside it", self.BELOW, {}),
            ("below it, staging below it", self.BELOW, {"STUB_IMPORTS": self.OWN_INTERNAL + "/derive"}),
        ]
        for name, leftover, env in cases:
            with self.subTest(name):
                self.write(leftover + "/doc.go", "package main\n")
                proc, calls = self.run_script("./cmd/tool", env=dict(env, STUB_PKG_NAME="main"))
                self.assert_refused_before_gremlins(proc, calls)
                self.assertEqual(self.of(calls, "test"), [])
                self.assertIn("gremlins: %s is already there" % leftover, proc.stderr)
                self.assertTrue(os.path.isfile(os.path.join(self.root, leftover, "doc.go")),
                                "the leftover was removed")
                other = self.BELOW if leftover == self.BESIDE else self.BESIDE
                self.assertFalse(os.path.exists(os.path.join(self.root, other)))
                shutil.rmtree(os.path.join(self.root, leftover))

    def test_an_interrupt_ends_the_run_and_removes_the_staged_copy(self):
        # `trap cleanup EXIT INT TERM` ran the cleanup and returned to the
        # script, which carried on against the copy it had just removed: a
        # signal during the baseline went on to start gremlins on a directory
        # that was gone, and one during gremlins let the run exit 0. A signal
        # sent to the script alone is what tells a returning handler from one
        # that ends the run, since the child it waits for finishes normally.
        # A copy staged below the package is removed the same way, and the
        # package it sat in is left as it was.
        below = {"STUB_IMPORTS": self.OWN_INTERNAL + "/derive"}
        cases = [
            ("SIGTERM during the baseline", signal.SIGTERM, "test", 143, {}, self.BESIDE),
            ("SIGINT during gremlins", signal.SIGINT, "run", 130, {}, self.BESIDE),
            ("SIGTERM during the baseline of a copy below", signal.SIGTERM, "test", 143, below, self.BELOW),
            ("SIGINT during gremlins on a copy below", signal.SIGINT, "run", 130, below, self.BELOW),
        ]
        self.give_the_command_a_subtree()
        package = file_tree(os.path.join(self.root, "cmd", "tool"))
        for name, signum, during, status, env, where in cases:
            staged = os.path.join(self.root, where)
            with self.subTest(name):
                open(self.log, "w", encoding="utf-8").close()
                started = os.path.join(self.scratch, "started")
                if os.path.exists(started):
                    os.remove(started)
                run_env = {k: v for k, v in os.environ.items()
                           if not k.startswith("GREMLINS_") and k != "GOFLAGS" and not k.startswith("STUB_")}
                run_env.update({
                    "PATH": self.bin + os.pathsep + os.environ.get("PATH", ""),
                    "STUB_LOG": self.log,
                    "STUB_ROOT": self.root,
                    "STUB_PKG_NAME": "main",
                    "STUB_STARTED": started,
                    "STUB_STARTED_ON": during,
                    "STUB_TEST_SLEEP" if during == "test" else "STUB_GREMLINS_SLEEP": "1",
                })
                run_env.update(env)
                proc = subprocess.Popen([SCRIPT, "./cmd/tool"], cwd=self.root, env=run_env,
                                        stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
                deadline = time.monotonic() + 60
                while not os.path.exists(started) and time.monotonic() < deadline:
                    time.sleep(0.02)
                self.assertTrue(os.path.exists(started), "the stand-in was never reached")
                self.assertTrue(os.path.isdir(staged))
                proc.send_signal(signum)
                out, err = proc.communicate(timeout=60)
                self.assertEqual(proc.returncode, status, out + err)
                self.assertFalse(os.path.exists(staged), "the staged copy was left behind")
                self.assertEqual(file_tree(os.path.join(self.root, "cmd", "tool")), package)
                with open(self.log, encoding="utf-8") as fh:
                    calls = [json.loads(line) for line in fh]
                runs = self.of(calls, "run")
                if during == "test":
                    self.assertEqual(runs, [], "gremlins was started after the signal")
                else:
                    self.assertEqual(len(runs), 1)
                    self.assertTrue(runs[0]["target_exists"])

    @staticmethod
    def trimpath_hint(proc):
        """The one line of the refusal that offers a command to reproduce it."""
        lines = [line for line in proc.stderr.splitlines()
                 if line.startswith("gremlins: every go command here runs under -trimpath")]
        return lines[0] if len(lines) == 1 else None

    def test_failing_baseline_is_refused(self):
        # The hint names -trimpath only where the suite failed in a run this
        # script compiled, and names the command that reproduces it: the
        # GOROOT and GOFLAGS the script ran under, and the pattern.
        trimpath = "every go command here runs under -trimpath (issue 1029)"
        run_as = "GOROOT=/stub/goroot GOFLAGS='-trimpath -count=1' go test "
        cases = [
            ("the first run fails", "1", "does not pass its own tests",
             run_as + "./internal/pkg/..., run from the module root, shows"),
            # gremlins reads a failing run of its per-mutant command as a
            # killed mutant, and a suite can pass under -cover and fail
            # without it.
            ("the per-mutant command fails", "2",
             "fails go test -failfast ./internal/pkg, the command gremlins runs against each mutant",
             run_as + "./internal/pkg, run from the module root, shows"),
            # A suite that passes once and fails the next time decides
            # nothing: a mutant's verdict would be a coin toss.
            ("the timed run fails", "3", "failed them on the timed run", None),
        ]
        for name, run, says, hint in cases:
            with self.subTest(name):
                proc, calls = self.run_script("./internal/pkg", env={"STUB_TEST_FAIL_RUN": run})
                self.assert_refused_before_gremlins(proc, calls)
                self.assertIn(says, proc.stderr)
                self.assertIn("--- FAIL: TestPlanted", proc.stderr)
                self.assertEqual(len(self.of(calls, "test")), int(run), "a run went on after the one that failed")
                self.assertEqual(trimpath in proc.stderr, hint is not None, proc.stderr)
                if hint is not None:
                    self.assertIn(hint, proc.stderr)
        # A staged copy that fails where it was staged is refused with a
        # reason of its own, since the verdicts would be about the staging,
        # and the copy is removed all the same (issue 872).
        # A copy staged below the package is one level deeper than the tests
        # were written for, so the refusal names the relative path a test
        # climbs out by as the likely cause there, and only there.
        climbs = ("so a test that climbs out of its directory with a relative path such as ../.."
                  " lands one level short")
        for name, env, staged in (("beside", {}, self.BESIDE),
                                  ("below", {"STUB_IMPORTS": self.OWN_INTERNAL + "/derive"}, self.BELOW)):
            with self.subTest("the staged copy fails", staged=name):
                proc, calls = self.run_script("./cmd/tool",
                                              env=dict(env, STUB_PKG_NAME="main", STUB_TEST_FAIL_RUN="1"))
                self.assert_refused_before_gremlins(proc, calls)
                self.assertIn("the staged copy of ./cmd/tool does not pass its own tests there", proc.stderr)
                self.assertIn("--- FAIL: TestPlanted", proc.stderr)
                self.assertIn(trimpath, proc.stderr)
                self.assertEqual(climbs in proc.stderr, name == "below", proc.stderr)
                self.assertEqual(self.baselines(calls)[0]["argv"][-1], "./" + staged + "/...")
                self.assertFalse(os.path.exists(os.path.join(self.root, staged)))
        # The command offered to reproduce a failure is the one the script
        # ran: the caller's GOFLAGS in front of its own, the toolchain root,
        # the tags, and the package where it is. A staged copy is gone by the
        # time anyone reads the hint, so the hint never names it.
        with self.subTest("the hint carries the tags, GOROOT and GOFLAGS, and names the original package"):
            proc, calls = self.run_script("./cmd/tool", flags="--tags e2e", env={
                "STUB_PKG_NAME": "main", "STUB_NEEDS_TAG": "e2e", "STUB_TEST_FAIL_RUN": "1",
                "STUB_GOROOT": "/toolchain/root", "GOFLAGS": "-mod=mod"})
            self.assert_refused_before_gremlins(proc, calls)
            self.assertEqual(
                self.trimpath_hint(proc),
                "gremlins: every go command here runs under -trimpath (issue 1029), which hands a test"
                " that locates files through runtime.Caller a module-relative path;"
                " GOROOT=/toolchain/root GOFLAGS='-mod=mod -trimpath -count=1' go test -tags e2e"
                " ./cmd/tool/..., run from the module root, shows whether that is the cause")
        with self.subTest("the per-mutant hint of a staged copy names the original package"):
            proc, calls = self.run_script("./cmd/tool", env={"STUB_PKG_NAME": "main", "STUB_TEST_FAIL_RUN": "2"})
            self.assert_refused_before_gremlins(proc, calls)
            self.assertIn("fails go test -failfast ./cmd/tool.mutants-main,", proc.stderr)
            self.assertIn(run_as + "./cmd/tool, run from the module root", self.trimpath_hint(proc))

    def test_gremlins_runs_under_count_1_with_invert_logical_and_the_default_exclusion(self):
        cases = [
            ("", {}, True),
            ("-E x", {}, False),
            ("--exclude-files=x", {}, False),
            ("--exclude-files x", {}, False),
            # gremlins reads `_` and `.` in a flag name as `-`.
            ("--exclude_files x", {}, False),
            ("-dE x", {}, False),
            ("", {"GREMLINS_UNLEASH_EXCLUDE_FILES": "x"}, False),
            # The root command's --silent and cobra's --help are switches
            # gremlins accepts in either spelling, so they pass through.
            ("-s", {}, True),
            ("-h", {}, True),
            ("-sdE x", {}, False),
            # The value-taking shorthands, in every spelling pflag reads a
            # value in: the next word, joined to the letter, or behind a
            # switch in a cluster. SKILL.md's own example is `-S l`.
            ("-S l", {}, True),
            ("-Sl", {}, True),
            ("-dS l", {}, True),
            ("-D main", {}, True),
            ("-o out.json", {}, True),
        ]
        for flags, env, default in cases:
            with self.subTest(flags=flags, env=env):
                proc, calls = self.run_script("./internal/pkg", flags=flags, env=env)
                self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
                run = self.gremlins(proc, calls)
                self.assertIn("-count=1", run["goflags"].split())
                self.assertIn("-trimpath", run["goflags"].split())
                argv = run["argv"]
                self.assertEqual(argv[2:6], ["unleash", "--invert-logical", "--workers", "4"])
                self.assertEqual(argv[-1], "./internal/pkg")
                self.assertEqual("--exclude-files=/" in argv, default, argv)
                if flags:
                    self.assertEqual(argv[8:-1][:len(flags.split())], flags.split())

    def test_every_go_command_runs_under_trimpath_with_the_toolchains_root(self):
        # gremlins copies the module into a directory per worker, and without
        # -trimpath the package's directory is part of every compile key, so
        # nothing compiled here would be of use to a worker (issue 1029). The
        # flag has to reach every go test here as well as gremlins, or the
        # cache the per-mutant command leaves is keyed on the wrong flags; the
        # caller's own GOFLAGS stay in front of it. -trimpath also erases the
        # toolchain root a test binary falls back to, so GOROOT carries the
        # one `go env GOROOT` names to everything after it.
        cases = [
            ("no GOFLAGS of the caller's", {}, []),
            ("the caller's GOFLAGS", {"GOFLAGS": "-mod=mod"}, ["-mod=mod"]),
        ]
        for name, env, kept in cases:
            with self.subTest(name):
                proc, calls = self.run_script("./internal/pkg", env=dict(env, STUB_GOROOT="/toolchain/root"))
                self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
                self.assertEqual(calls[0]["argv"], ["env", "GOROOT"], "the root was not the first thing asked")
                later = calls[1:]
                self.assertEqual(len(self.of(later, "test")), 3)
                self.assertEqual(len(self.of(later, "run")), 1)
                for call in later:
                    self.assertEqual(call["goflags"].split(), kept + ["-trimpath", "-count=1"], call["argv"])
                    self.assertEqual(call["goroot"], "/toolchain/root", call["argv"])

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
        # bash formats `time` with the locale's decimal separator. Under such
        # a locale the base would read 0,940, which the script's
        # positive-number check refuses, so every run would stop; without
        # the check awk would read it as 0, the coefficient and the ceiling's
        # cap would both hit the 6000 clamp and the ceiling would bound
        # nothing, and gawk would fail on the division outright.
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
