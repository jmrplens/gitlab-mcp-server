#!/usr/bin/env python3
"""Tests scripts/coverage-conditions.sh, the recipe behind `make coverage-conditions`.

gobco parses every file of a directory whose name ends in .go and
type-checks them together, whatever a //go:build line or a _windows.go name
says, so a package that declares a function once per platform stops with a
redeclaration panic before anything is measured (issue 1017). That was
cmd/server, internal/toolutil and test/e2e/internal/harness. The script
stages such a package in a copy of the module, under the temporary
directory, holding only the files the go command builds here, with the
constraint lines of the files it keeps blanked, and runs gobco there.

These cases drive the real script against a stand-in `go` placed first on
PATH. It answers `go env` and `go list` from the fixture tree, deciding
which files build the way the go command does (a GOOS name suffix, a
//go:build line over linux/amd64 and the tags a call passes, a leading dot
or underscore), and stands in for gobco itself: it records the directory it
was run in, its arguments and the .go files it would parse there, and it
panics the way gobco does when that directory holds a file its build context
would not build. What is asserted is what gobco would see: the directory it
runs in, the files in it, their constraint lines and positions, the tags it
passes on, the files the run names as left out, the refusals, and that the
copy is gone afterwards whatever happened.

Run with:

    python3 -m unittest discover -s scripts -p 'coverage_conditions_sh_test.py'
"""

import hashlib
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
SCRIPT = os.path.join(ROOT, "scripts", "coverage-conditions.sh")
MAKEFILE = os.path.join(ROOT, "Makefile")
CI_WORKFLOW = os.path.join(ROOT, ".github", "workflows", "ci.yml")
SPEC_CONDITIONS = os.path.join(ROOT, "scripts", "check-spec-conditions.sh")
GOBCO = "github.com/rillig/gobco@v1.3.4"

# Stands in for the go command and for gobco. Configured through STUB_*
# variables, which the script passes through untouched, and appends one JSON
# line per call to STUB_LOG. Its interpreter line is written in setUp,
# naming the interpreter running these tests with -S, so that no site
# initialisation slows every call down.
STUB_GO = r'''import json
import os
import sys
import time

args = sys.argv[1:]
env = os.environ
GOBCO = "github.com/rillig/gobco@v1.3.4"
# The platform this call is for: GOOS and GOARCH as the go command reads them,
# linux/amd64 when neither is set.
GOOS = env.get("GOOS", "linux")
GOARCH = env.get("GOARCH", "amd64")
# What that platform satisfies without being asked, as far as the fixtures go.
ACTIVE = {GOOS, GOARCH, "gc", "go1.22"} | ({"unix"} if GOOS != "windows" else set())
# The operating systems a file name can end in, which the fixtures use.
SYSTEMS = ("linux", "windows", "darwin")


def tags_of(argv):
    tags = set()
    for i, arg in enumerate(argv):
        value = None
        if arg == "-tags" and i + 1 < len(argv):
            value = argv[i + 1]
        elif arg.startswith("-tags="):
            value = arg[len("-tags="):]
        elif arg.startswith("-test=-tags="):
            value = arg[len("-test=-tags="):]
        if value is not None:
            tags = {tag for tag in value.split(",") if tag}
    return tags


def builds(path, tags):
    """Whether the go command builds the file under this call's platform and
    tags: a leading dot or underscore never, a GOOS suffix only for that
    system, and a //go:build line of terms joined by && when every term
    holds."""
    name = os.path.basename(path)
    if name.startswith((".", "_")):
        return False
    stem = name[:-len(".go")]
    if stem.endswith("_test"):
        stem = stem[:-len("_test")]
    if any(stem.endswith("_" + system) for system in SYSTEMS if system != GOOS):
        return False
    with open(path, encoding="utf-8") as fh:
        for line in fh:
            if line.startswith("package "):
                return True
            if line.startswith("//go:build "):
                for term in line[len("//go:build "):].split("&&"):
                    term = term.strip()
                    wanted = not term.startswith("!")
                    if (term.lstrip("!") in ACTIVE or term.lstrip("!") in tags) != wanted:
                        return False
                return True
    return True


def go_files(directory):
    """Every regular file whose name ends in .go, as go/parser.ParseDir
    reads a directory: a leading dot or underscore is no exception."""
    return sorted(name for name in os.listdir(directory)
                  if name.endswith(".go") and os.path.isfile(os.path.join(directory, name)))


def clause(path):
    with open(path, encoding="utf-8") as fh:
        for line in fh:
            if line.startswith("package "):
                return line.split()[1]
    return ""


def header_constraint(path):
    with open(path, encoding="utf-8") as fh:
        for line in fh:
            if line.startswith("package "):
                return False
            if line.startswith("//go:build ") or line.startswith("// +build "):
                return True
    return False


def render(template, fields):
    for placeholder, value in fields:
        template = template.replace(placeholder, value)
    if "{{" in template:
        sys.stderr.write("stub go list cannot render " + template + "\n")
        sys.exit(2)
    return template


entry = {"argv": args, "cwd": os.getcwd(), "goos": GOOS, "goarch": GOARCH}
status = 0
if args == ["env", "GOOS", "GOARCH"]:
    print(GOOS)
    print(GOARCH)
elif args == ["tool", "dist", "list"]:
    # A slice of what the toolchain prints, one os/arch pair per line.
    for pair in ("aix/ppc64", "android/arm64", "darwin/amd64", "darwin/arm64", "freebsd/amd64",
                 "js/wasm", "linux/386", "linux/amd64", "linux/arm64", "openbsd/amd64",
                 "windows/amd64", "windows/arm64"):
        print(pair)
elif args[:2] == ["list", "-e"]:
    tags = tags_of(args)
    pkgdir = os.path.normpath(os.path.join(os.getcwd(), args[-1]))
    exists = os.path.isdir(pkgdir)
    names = [n for n in go_files(pkgdir) if builds(os.path.join(pkgdir, n), tags)] if exists else []
    tests = [n for n in names if n.endswith("_test.go") and not clause(os.path.join(pkgdir, n)).endswith("_test")]
    xtests = [n for n in names if n.endswith("_test.go") and clause(os.path.join(pkgdir, n)).endswith("_test")]
    gofiles = [n for n in names if not n.endswith("_test.go")]
    error = env.get("STUB_LIST_ERROR", "")
    if env.get("STUB_LIST_ERROR_FOR_GOOS") == GOOS:
        error = "planted load error for " + GOOS
    if not exists:
        error = "directory %s outside main module or its selected dependencies" % args[-1]
    elif not gofiles and not tests and not xtests:
        error = "build constraints exclude all Go files in " + pkgdir
    shown = pkgdir
    separator = env.get("STUB_DIR_SEPARATOR")
    if separator and pkgdir != env["STUB_ROOT"]:
        below = os.path.relpath(pkgdir, env["STUB_ROOT"])
        shown = env["STUB_ROOT"] + separator + below.replace("/", separator)
    print(render(args[args.index("-f") + 1], [
        ("{{with .Module}}{{.Dir}}{{end}}", env["STUB_ROOT"]),
        ("{{.Dir}}", shown if exists else ""),
        ("{{len .TestGoFiles}}", str(len(tests))),
        ("{{len .XTestGoFiles}}", str(len(xtests))),
        ('{{join .GoFiles " "}}', " ".join(gofiles)),
        ('{{join .CgoFiles " "}}', ""),
        ('{{join .TestGoFiles " "}}', " ".join(tests)),
        ('{{join .XTestGoFiles " "}}', " ".join(xtests)),
        ("{{with .Error}}{{.}}{{end}}", error),
    ]))
elif args[:2] == ["run", GOBCO]:
    here = os.getcwd()
    tags = tags_of(args)
    if env.get("STUB_STARTED"):
        open(env["STUB_STARTED"], "w", encoding="utf-8").close()
    files = go_files(here)
    entry["files"] = files
    entry["constrained"] = [n for n in files if header_constraint(os.path.join(here, n))]
    entry["contents"] = {}
    for name in files:
        with open(os.path.join(here, name), encoding="utf-8") as fh:
            entry["contents"][name] = fh.read()
    entry["others"] = sorted(
        os.path.relpath(os.path.join(d, n), here)
        for d, _, ns in os.walk(here) for n in ns if not n.endswith(".go"))
    # The module gobco would copy and run in: the nearest directory above
    # holding a go.mod, and every file in it, which is what the package's
    # tests can reach by a relative path and what its imports resolve to.
    root = here
    while not os.path.exists(os.path.join(root, "go.mod")) and os.path.dirname(root) != root:
        root = os.path.dirname(root)
    entry["module_root"] = root
    entry["module_files"] = sorted(
        os.path.relpath(os.path.join(d, n), root).replace(os.sep, "/")
        for d, _, ns in os.walk(root) for n in ns)
    unbuilt = [n for n in files if not builds(os.path.join(here, n), tags)]
    if unbuilt:
        # What gobco does with a directory holding a file the go command
        # leaves out: it type-checks it with the rest, and a platform pair
        # declares the same function twice.
        print("panic: %s:5:6: f redeclared in this block" % unbuilt[0])
        status = 2
    else:
        time.sleep(float(env.get("STUB_GOBCO_SLEEP", "0")))
        figure = env.get("STUB_GOBCO_REPORT", "3/4")
        if not any(n.endswith("_test.go") for n in files):
            # No test binary ran, so no counts were written, and gobco
            # reports nothing measured rather than conditions unevaluated.
            figure = "0/0"
        print("")
        if figure != "none":
            print("Condition coverage: " + figure)
            # The conditions it names, one per line, which a case sets with
            # STUB_GOBCO_CONDITIONS (an empty value names none), or one in
            # the first file by default.
            if "STUB_GOBCO_CONDITIONS" in env:
                for line in env["STUB_GOBCO_CONDITIONS"].split("\n"):
                    if line:
                        print(line)
            else:
                print('%s:4:9: condition "c == 1" was once true but never false' % files[0])
        status = int(env.get("STUB_GOBCO_STATUS", "0"))
else:
    sys.stderr.write("stub go: unexpected call %r\n" % (args,))
    status = 2

entry["status"] = status
with open(env["STUB_LOG"], "a", encoding="utf-8") as fh:
    fh.write(json.dumps(entry) + "\n")
sys.exit(status)
'''

FILES = {
    # A package the go command builds whole: gobco reads it where it is.
    "internal/plain/doc.go": "// Package plain is a fixture.\npackage plain\n",
    "internal/plain/plain.go": "package plain\n\nfunc f(c int) bool {\n\treturn c == 1\n}\n",
    "internal/plain/plain_test.go": "package plain\n\nimport \"testing\"\n\nfunc TestF(t *testing.T) { f(1) }\n",
    # The cmd/server shape: a function declared once per platform, a test
    # file per platform, and files the tests read beside them.
    "cmd/tool/main.go": "package main\n\nfunc main() {}\n",
    "cmd/tool/proc_unix.go": "//go:build !windows\n\npackage main\n\nfunc g(c int) bool {\n\treturn c == 1\n}\n",
    "cmd/tool/proc_windows.go": "//go:build windows\n\npackage main\n\nfunc g(c int) bool {\n\treturn c == 2\n}\n",
    "cmd/tool/main_test.go": "package main\n\nimport \"testing\"\n\nfunc TestG(t *testing.T) { g(1) }\n",
    "cmd/tool/proc_windows_test.go": "//go:build windows\n\npackage main\n",
    "cmd/tool/proc_unix_test.go": "// Copyright line above the constraint.\n\n//go:build unix\n// +build unix\n\npackage main\n",
    "cmd/tool/embedded.txt": "embedded\n",
    "cmd/tool/testdata/fixture.txt": "fixture\n",
    # Files the go command builds under a constraint gobco's own context
    # rejects, in either spelling, beside one on the platform alone that it
    # reads as go does, and nothing it leaves out.
    "internal/release/release.go": "//go:build go1.22\n\npackage release\n\nfunc h(c int) bool {\n\treturn c == 1\n}\n",
    "internal/release/legacy.go": "// +build linux,integration\n\npackage release\n",
    "internal/release/platform.go": "//go:build linux && amd64\n\npackage release\n",
    "internal/release/release_test.go": "package release\n\nimport \"testing\"\n\nfunc TestH(t *testing.T) { h(1) }\n",
    # A constraint on the platform alone and no counterpart left out: gobco
    # reads this package as the go command does, where it is.
    "internal/unixonly/stream.go": "package unixonly\n\nfunc u(c int) bool {\n\treturn c == 1\n}\n",
    "internal/unixonly/stream_unix_test.go": "//go:build !windows\n\npackage unixonly\n\nimport \"testing\"\n\nfunc TestU(t *testing.T) { u(1) }\n",
    # Files the go command never builds, whatever the platform.
    "internal/scratch/scratch.go": "package scratch\n\nfunc k() bool { return true }\n",
    "internal/scratch/scratch_test.go": "package scratch\n",
    # Named so that C and a dictionary collation order them differently.
    "internal/scratch/_a_draft.go": "package scratch\n\nfunc k() bool { return false }\n",
    "internal/scratch/.b_backup.go": "package scratch\n\nfunc k() bool { return false }\n",
    # The e2e harness shape: every file behind a tag, and a pair split on the
    # race detector.
    "test/tagged/doc.go": "// Package tagged is a fixture.\npackage tagged\n",
    "test/tagged/harness.go": "//go:build e2e\n\npackage tagged\n",
    "test/tagged/harness_test.go": "//go:build e2e\n\npackage tagged\n",
    "test/tagged/server_race.go": "//go:build e2e && race\n\npackage tagged\n\nfunc s() {}\n",
    "test/tagged/server_norace.go": "//go:build e2e && !race\n\npackage tagged\n\nfunc s() {}\n",
    # No test at all, and a platform half left out.
    "internal/untested/untested.go": "package untested\n",
    "internal/untested/untested_windows.go": "package untested\n\nfunc w() {}\n",
    # A package only Windows builds a file of.
    "internal/winonly/w_windows.go": "package winonly\n\nfunc w(c int) bool {\n\treturn c == 1\n}\n",
    "internal/winonly/w_windows_test.go": "package winonly\n",
    # What the rest of the module holds: files a test reads by a relative
    # path, which the staged copy keeps, and the directories it leaves out.
    "docs/guide.md": "guide\n",
    "docs/dist/kept.md": "a dist below the root is not the build output\n",
    "server.json": "{}\n",
    "site/page.md": "page\n",
    ".claude/settings.json": "{}\n",
    ".git/HEAD": "ref: refs/heads/main\n",
    ".claude/worktrees/other/stray.go": "package stray\n",
    "node_modules/pkg/index.js": "module.exports = 1\n",
    "site/node_modules/pkg/index.js": "module.exports = 1\n",
    "dist/gitlab-mcp-server": "binary\n",
}

# The module's files a staged copy keeps and leaves out.
KEPT = ["docs/guide.md", "docs/dist/kept.md", "server.json", "site/page.md", ".claude/settings.json", "go.mod"]
LEFT_OUT = [".git/HEAD", ".claude/worktrees/other/stray.go", "node_modules/pkg/index.js",
            "site/node_modules/pkg/index.js", "dist/gitlab-mcp-server"]


def dictionary_locale():
    """An installed locale whose collation ignores punctuation at its first
    level, so that it orders "_a" before ".b" where C orders them the other
    way round, or None when there is none."""
    if shutil.which("locale") is None:
        return None
    listed = subprocess.run(["locale", "-a"], capture_output=True, text=True, check=False).stdout.split()
    for name in listed:
        if not name.lower().startswith("en_us.utf"):
            continue
        probe = subprocess.run(["bash", "-c", "printf '%s\\n' .b _a | sort"], capture_output=True, text=True,
                               check=False, env=dict(os.environ, LC_ALL=name))
        if probe.stdout.split() == ["_a", ".b"]:
            return name
    return None


class CoverageConditionsTest(unittest.TestCase):

    def setUp(self):
        # The real path, since the stand-in compares what it is run in with
        # directories it reads from os.getcwd(), which resolves links.
        self.scratch = os.path.realpath(tempfile.mkdtemp(prefix="coverage-conditions-"))
        self.addCleanup(shutil.rmtree, self.scratch, True)
        self.root = os.path.join(self.scratch, "module")
        for rel, content in FILES.items():
            path = os.path.join(self.root, rel)
            os.makedirs(os.path.dirname(path), exist_ok=True)
            with open(path, "w", encoding="utf-8") as fh:
                fh.write(content)
        with open(os.path.join(self.root, "go.mod"), "w", encoding="utf-8") as fh:
            fh.write("module example.com/m\n\ngo 1.27\n")
        self.bin = os.path.join(self.scratch, "bin")
        os.makedirs(self.bin)
        stub = os.path.join(self.bin, "go")
        with open(stub, "w", encoding="utf-8") as fh:
            fh.write("#!" + sys.executable + " -S\n" + STUB_GO)
        os.chmod(stub, 0o755)
        self.log = os.path.join(self.scratch, "go.log")
        # The script's temporary directory, so that what it stages there and
        # whether it removed it can both be seen.
        self.tmp = os.path.join(self.scratch, "tmp")
        os.makedirs(self.tmp)

    def environment(self, env=None):
        run_env = {k: v for k, v in os.environ.items() if not k.startswith("STUB_")}
        run_env.update({
            "PATH": self.bin + os.pathsep + os.environ.get("PATH", ""),
            "STUB_LOG": self.log,
            "STUB_ROOT": self.root,
            "TMPDIR": self.tmp,
        })
        run_env.update(env or {})
        return run_env

    def calls(self):
        with open(self.log, encoding="utf-8") as fh:
            return [json.loads(line) for line in fh]

    def run_script(self, *args, env=None):
        """Runs the script from the module root, the way make does, and
        returns the finished process with the go calls it made."""
        open(self.log, "w", encoding="utf-8").close()
        proc = subprocess.run([SCRIPT, *args], cwd=self.root, env=self.environment(env),
                              capture_output=True, text=True, timeout=60, check=False)
        return proc, self.calls()

    @staticmethod
    def gobco(calls):
        return [c for c in calls if c["argv"][:1] == ["run"]]

    @staticmethod
    def lists(calls):
        return [c for c in calls if c["argv"][:1] == ["list"]]

    def snapshot(self, rel):
        """Every file below a package directory with a digest of its bytes."""
        found = {}
        base = os.path.join(self.root, rel)
        for directory, _, names in os.walk(base):
            for name in names:
                path = os.path.join(directory, name)
                with open(path, "rb") as fh:
                    found[os.path.relpath(path, base)] = hashlib.sha256(fh.read()).hexdigest()
        return found

    def staged_copies(self):
        """What the script left in its temporary directory: the staged module
        and the captured report, both of which it removes whatever happens."""
        return sorted(os.listdir(self.tmp))

    def assert_staged(self, run, rel):
        """gobco ran on the package inside a copy of the module under the
        temporary directory: at the same path below a root of the same name,
        with the rest of the module beside it and none of what the copy
        leaves out, so an import path, a relative path and a directory name
        are all the original's."""
        self.assertTrue(run["cwd"].startswith(self.tmp + os.sep), run["cwd"])
        root = run["module_root"]
        self.assertEqual(os.path.basename(root), os.path.basename(self.root))
        self.assertEqual(os.path.relpath(run["cwd"], root), rel)
        for path in KEPT:
            self.assertIn(path, run["module_files"])
        for path in LEFT_OUT:
            self.assertNotIn(path, run["module_files"])

    def assert_blanked_line_for_line(self, run, rel, blanked):
        """Every file gobco read in the copy is the original with every other
        line where it was, so a position gobco reports is one in the original
        file, and with its constraint lines blank exactly when it is one of
        the files named: those whose constraint names more than the platform.
        A constraint on the platform alone is kept, since gobco reads it as
        the go command does."""
        for name, staged in run["contents"].items():
            with self.subTest(name=name):
                with open(os.path.join(self.root, rel, name), encoding="utf-8") as fh:
                    original = fh.read().split("\n")
                copied = staged.split("\n")
                self.assertEqual(len(copied), len(original))
                for number, (was, now) in enumerate(zip(original, copied), 1):
                    if was.startswith(("//go:build ", "// +build ")) and name in blanked:
                        self.assertEqual(now, "", "line %d" % number)
                    else:
                        self.assertEqual(now, was, "line %d" % number)

    def test_package_the_go_command_builds_whole_is_measured_where_it_is(self):
        # With tags or without, since tags change nothing about a package
        # with no constraint; they still reach gobco's go test.
        cases = [
            ("no tags", (), ["run", GOBCO]),
            ("tags", ("e2e",), ["run", GOBCO, "-test=-tags=e2e"]),
        ]
        for name, extra, argv in cases:
            with self.subTest(name):
                before = self.snapshot("internal/plain")
                proc, calls = self.run_script("./internal/plain", *extra)
                self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
                runs = self.gobco(calls)
                self.assertEqual(len(runs), 1, calls)
                self.assertEqual(runs[0]["argv"], argv)
                self.assertEqual(runs[0]["cwd"], os.path.join(self.root, "internal", "plain"))
                self.assertEqual(runs[0]["files"], ["doc.go", "plain.go", "plain_test.go"])
                # gobco's own report reaches the caller untouched, and nothing
                # is said about a staging that did not happen.
                self.assertIn("Condition coverage: 3/4", proc.stdout)
                self.assertNotIn("gobco:", proc.stdout + proc.stderr)
                self.assertEqual(self.snapshot("internal/plain"), before)
                self.assertEqual(self.staged_copies(), [])

    def test_platform_pair_is_measured_through_a_copy_of_what_builds_here(self):
        before = self.snapshot("")
        proc, calls = self.run_script("./cmd/tool")
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        runs = self.gobco(calls)
        self.assertEqual(len(runs), 1, calls)
        run = runs[0]
        # One build context: gobco runs on the package inside a copy of the
        # module, where every import path, relative path and directory name
        # is the original's, on the files the go command builds here and no
        # other.
        self.assert_staged(run, os.path.join("cmd", "tool"))
        self.assertEqual(run["files"], ["main.go", "main_test.go", "proc_unix.go", "proc_unix_test.go"])
        self.assertNotIn("panic", proc.stdout)
        self.assertEqual(run["others"], ["embedded.txt", os.path.join("testdata", "fixture.txt")])
        # The run names what it left out, under which context, and how it
        # measured.
        self.assertIn("(proc_windows.go proc_windows_test.go)", proc.stdout)
        self.assertIn("linux/amd64 under build tags (none)", proc.stdout)
        self.assertIn("staged copy of the module", proc.stdout)
        # Nothing claims the files left out are measured anywhere.
        self.assertIn("are not measured by this run", proc.stdout)
        self.assertNotIn("measured where they build", proc.stdout)
        self.assertIn("Condition coverage: 3/4", proc.stdout)
        # The kept halves are constrained on the platform alone (!windows,
        # and unix in both spellings), which gobco reads as go does, so their
        # lines stay and nothing is said about blanking.
        self.assertEqual(run["constrained"], ["proc_unix.go", "proc_unix_test.go"])
        self.assert_blanked_line_for_line(run, "cmd/tool", set())
        self.assertNotIn("blanks", proc.stdout)
        # The module itself is untouched, and the copy is gone.
        self.assertEqual(self.snapshot(""), before)
        self.assertEqual(self.staged_copies(), [])

    def test_the_same_package_run_where_it_is_panics_the_way_gobco_does(self):
        # What the recipe did before the script, reproduced against the
        # stand-in, so the cases above are held against the failure they
        # prevent.
        open(self.log, "w", encoding="utf-8").close()
        proc = subprocess.run([os.path.join(self.bin, "go"), "run", GOBCO],
                              cwd=os.path.join(self.root, "cmd", "tool"), env=self.environment(),
                              capture_output=True, text=True, timeout=60, check=False)
        self.assertEqual(proc.returncode, 2, proc.stdout + proc.stderr)
        self.assertIn("redeclared in this block", proc.stdout)

    def test_files_the_go_command_never_builds_are_left_out(self):
        # ParseDir reads a name beginning with a dot or an underscore, and the
        # go command never builds one; go list's IgnoredGoFiles does not list
        # one either, which is why the script takes the complement of what
        # go list says it builds.
        # The files are named in byte order whatever the caller's locale, and
        # a dictionary collation, where one is installed, would put the
        # underscore first.
        cases = [("the caller's locale", {})]
        locale_name = dictionary_locale()
        if locale_name is not None:
            cases.append((locale_name, {"LC_ALL": locale_name, "LANG": locale_name}))
        for name, env in cases:
            with self.subTest(name):
                proc, calls = self.run_script("./internal/scratch", env=env)
                self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
                run = self.gobco(calls)[0]
                self.assert_staged(run, os.path.join("internal", "scratch"))
                self.assertEqual(run["files"], ["scratch.go", "scratch_test.go"])
                self.assertIn("(.b_backup.go _a_draft.go)", proc.stdout)
                self.assertEqual(self.staged_copies(), [])

    def test_built_file_under_a_constraint_gobco_misjudges_is_staged_with_its_line_blanked(self):
        # gobco instruments a parsed file only where a go/build.Context
        # holding GOOS and GOARCH alone accepts it, which a release or a
        # custom tag is not, so the file would be compiled and its conditions
        # absent. A constraint on the platform alone is read the same way by
        # both and keeps its line.
        proc, calls = self.run_script("./internal/release")
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        run = self.gobco(calls)[0]
        self.assert_staged(run, os.path.join("internal", "release"))
        self.assertEqual(run["files"], ["legacy.go", "platform.go", "release.go", "release_test.go"])
        self.assert_blanked_line_for_line(run, "internal/release", {"legacy.go", "release.go"})
        self.assertEqual(run["constrained"], ["platform.go"])
        self.assertEqual(run["contents"]["release.go"].split("\n")[0], "")
        self.assertIn("builds legacy.go release.go under a build constraint naming more than the platform",
                      proc.stdout)
        self.assertNotIn("does not build", proc.stdout)
        self.assertEqual(self.staged_copies(), [])

    def test_constraint_on_the_platform_alone_is_measured_where_it_is(self):
        # gobco's context evaluates an operating system, an architecture and
        # unix exactly as the go command does, so a file behind //go:build
        # !windows with no Windows counterpart needs no copy of the module.
        before = self.snapshot("internal/unixonly")
        proc, calls = self.run_script("./internal/unixonly")
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        run = self.gobco(calls)[0]
        self.assertEqual(run["cwd"], os.path.join(self.root, "internal", "unixonly"))
        self.assertEqual(run["constrained"], ["stream_unix_test.go"])
        self.assertNotIn("gobco:", proc.stdout + proc.stderr)
        self.assertEqual(self.snapshot("internal/unixonly"), before)
        self.assertEqual(self.staged_copies(), [])

    def test_package_behind_a_tag_is_refused_without_it(self):
        proc, calls = self.run_script("./test/tagged")
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertEqual(self.gobco(calls), [])
        self.assertIn("keeps no test file", proc.stderr)
        self.assertIn("TAGS=<tag>", proc.stderr)
        self.assertEqual(self.staged_copies(), [])

    def test_package_behind_a_tag_is_measured_with_it(self):
        # go list is asked under the tags gobco's go test gets, so a file of
        # the package the tag admits is kept and only the other half of the
        # race pair is left out; the kept files are blanked, since gobco's
        # own context ignores a custom tag and would instrument none of them.
        proc, calls = self.run_script("./test/tagged", "e2e")
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        for call in self.lists(calls):
            self.assertIn("e2e", call["argv"][call["argv"].index("-tags") + 1].split(","), call["argv"])
        run = self.gobco(calls)[0]
        self.assertEqual(run["argv"], ["run", GOBCO, "-test=-tags=e2e"])
        self.assert_staged(run, os.path.join("test", "tagged"))
        self.assertEqual(run["files"], ["doc.go", "harness.go", "harness_test.go", "server_norace.go"])
        self.assertIn("(server_race.go)", proc.stdout)
        self.assertIn("linux/amd64 under build tags e2e", proc.stdout)
        self.assertEqual(run["constrained"], [])
        self.assert_blanked_line_for_line(run, "test/tagged", {"harness.go", "harness_test.go", "server_norace.go"})
        # Named in the order go list gives them, the package's files first.
        self.assertIn("the copy blanks the build constraint lines of harness.go server_norace.go harness_test.go",
                      proc.stdout)
        self.assertIn("Condition coverage: 3/4", proc.stdout)
        self.assertEqual(self.staged_copies(), [])

    def test_report_that_measured_no_condition_is_refused(self):
        # 0/0 is what gobco prints for a package no test wrote counts for and
        # for one whose every file it declined to instrument, and either reads
        # like a package with nothing left to test.
        cases = [
            ("0/0 where it is", "./internal/plain", {"STUB_GOBCO_REPORT": "0/0"}),
            ("no figure at all", "./internal/plain", {"STUB_GOBCO_REPORT": "none"}),
            ("a staged package with no test", "./internal/untested", {}),
        ]
        for name, pkg, env in cases:
            with self.subTest(name):
                proc, calls = self.run_script(pkg, env=env)
                self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
                self.assertEqual(len(self.gobco(calls)), 1)
                self.assertIn("measured no condition of " + pkg, proc.stderr)
                self.assertEqual(self.staged_copies(), [])

    def test_package_go_cannot_load_is_refused(self):
        cases = [
            ("a path that is not there", "./internal/missing", {}, "outside main module"),
            ("an error go list reports", "./internal/plain", {"STUB_LIST_ERROR": "planted load error"},
             "planted load error"),
        ]
        for name, pkg, env, says in cases:
            with self.subTest(name):
                proc, calls = self.run_script(pkg, env=env)
                self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
                self.assertEqual(self.gobco(calls), [])
                self.assertIn("go cannot load " + pkg, proc.stderr)
                self.assertIn(says, proc.stderr)

    def test_module_root_package_is_staged_like_any_other(self):
        # The copy is of the module, so the root package is staged at the
        # root of the copy, where a copy of the package alone could not have
        # been put anywhere inside the module.
        for rel, content in (("root.go", "package m\n"), ("root_windows.go", "package m\n"),
                             ("root_test.go", "package m\n")):
            with open(os.path.join(self.root, rel), "w", encoding="utf-8") as fh:
                fh.write(content)
        proc, calls = self.run_script(".")
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        run = self.gobco(calls)[0]
        self.assert_staged(run, ".")
        self.assertEqual(run["files"], ["root.go", "root_test.go"])
        self.assertIn("(root_windows.go)", proc.stdout)
        self.assertEqual(self.staged_copies(), [])

    def test_gobco_status_is_kept_and_a_failed_run_says_its_figures_are_not_a_measurement(self):
        # gobco prints its figures when its go test fails, so every failed run
        # says in one line that they are not a measurement. A staged run also
        # names what the staging changed: the blanked constraint lines first,
        # where there are any, since a test that reads its own headers (the
        # e2e harness's source checks) fails on exactly those.
        blanked = "the copy blanked the build constraint lines of harness.go server_norace.go harness_test.go"
        cases = [
            ("where it is", ("./internal/plain",), "gobco exited 1 on ./internal/plain, so", False),
            ("staged, nothing blanked", ("./cmd/tool",), "gobco exited 1 on the staged copy of ./cmd/tool", False),
            ("staged and blanked", ("./test/tagged", "e2e"), "gobco exited 1 on the staged copy of ./test/tagged",
             True),
        ]
        for name, args, says, names_blanked in cases:
            with self.subTest(name):
                proc, calls = self.run_script(*args, env={"STUB_GOBCO_STATUS": "1"})
                self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
                self.assertEqual(len(self.gobco(calls)), 1)
                self.assertIn("Condition coverage: 3/4", proc.stdout)
                self.assertIn(says, proc.stderr)
                self.assertIn("so any figure above is not a measurement", proc.stderr)
                self.assertEqual(blanked in proc.stderr, names_blanked, proc.stderr)
                self.assertEqual("finds them blank there" in proc.stderr, names_blanked, proc.stderr)
                self.assertEqual("the copy leaves out" in proc.stderr, args[0] != "./internal/plain", proc.stderr)
                self.assertEqual(self.staged_copies(), [])

    def test_windows_separators_from_go_list_find_the_package_in_the_copy(self):
        # go list prints native paths, and a strip expecting a slash strips
        # nothing on a backslash: gobco would be pointed at a directory the
        # copy does not have.
        proc, calls = self.run_script("./cmd/tool", env={"STUB_DIR_SEPARATOR": "\\"})
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assert_staged(self.gobco(calls)[0], os.path.join("cmd", "tool"))
        self.assertEqual(self.staged_copies(), [])

    def test_without_a_gate_a_one_way_condition_is_reported_and_passes(self):
        # The default: the report is printed whole and the run passes whatever
        # it found, which is what every caller before the gate relied on.
        proc, _ = self.run_script("./internal/plain", env={
            "STUB_GOBCO_CONDITIONS": 'plain.go:4:9: condition "c == 1" was never evaluated'})
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assertIn('plain.go:4:9: condition "c == 1" was never evaluated', proc.stdout)
        self.assertNotIn("GOBCO_GATE", proc.stdout + proc.stderr)

    def test_gate_all_fails_on_every_condition_not_evaluated_both_ways(self):
        # The three shapes gobco writes such a condition in, a line a Windows
        # console ended with a carriage return among them, and a report that
        # names none, which passes and says so.
        scope = "the files of ./internal/plain measured on linux/amd64 under build tags (none)"
        cases = [
            ("never false", 'plain.go:4:9: condition "c == 1" was 3 times true but never false', 1),
            ("never true", 'plain.go:4:9: condition "c == 1" was once false but never true', 1),
            ("never evaluated", 'plain.go:4:9: condition "c == 1" was never evaluated', 1),
            ("a line ending in a carriage return", 'plain.go:4:9: condition "c == 1" was never evaluated\r', 1),
            ("every condition both ways", 'plain.go:4:9: condition "c == 1" was once true and once false', 0),
            ("no condition named", "", 0),
        ]
        for name, conditions, status in cases:
            with self.subTest(name):
                proc, calls = self.run_script("./internal/plain", env={
                    "GOBCO_GATE": "all", "STUB_GOBCO_CONDITIONS": conditions})
                self.assertEqual(proc.returncode, status, proc.stdout + proc.stderr)
                self.assertEqual(len(self.gobco(calls)), 1)
                if status:
                    self.assertIn("GOBCO_GATE=all: 1 condition(s) in %s were not evaluated both ways:" % scope,
                                  proc.stderr)
                    self.assertIn("  " + conditions.rstrip("\r"), proc.stderr)
                else:
                    self.assertIn("GOBCO_GATE=all: every condition in %s was evaluated both ways" % scope, proc.stdout)
                    self.assertNotIn("left to a run there", proc.stdout)
                self.assertEqual(self.staged_copies(), [])

    def test_gate_beyond_holds_only_the_files_the_reference_does_not_build(self):
        # Measured as Windows, held against linux/amd64: the Windows half is
        # the one file only this run can measure, so a condition left one-way
        # there fails, under either separator, while one in a file Linux
        # builds as well is counted, named and passed.
        windows = {"GOOS": "windows", "GOBCO_GATE": "beyond:linux/amd64"}
        held = "the files of ./cmd/tool that windows/amd64 under build tags (none) builds and linux/amd64 does not (proc_windows.go)"
        cases = [
            ("one in the Windows half", 'proc_windows.go:5:9: condition "c == 2" was once true but never false', 1, 0),
            ("one named with a Windows path", 'cmd\\tool\\proc_windows.go:5:9: condition "c == 2" was never evaluated', 1, 0),
            ("one in a file Linux builds too", 'main.go:3:9: condition "x" was never evaluated', 0, 1),
            ("one of each", 'main.go:3:9: condition "x" was never evaluated\n'
                            'proc_windows.go:5:9: condition "c == 2" was once false but never true', 1, 1),
            ("none", "", 0, 0),
        ]
        for name, conditions, status, left in cases:
            with self.subTest(name):
                proc, calls = self.run_script("./cmd/tool", env=dict(windows, STUB_GOBCO_CONDITIONS=conditions))
                self.assertEqual(proc.returncode, status, proc.stdout + proc.stderr)
                # The reference is asked for under its own platform, and the
                # run itself under this one.
                self.assertEqual([c["goos"] for c in self.lists(calls)], ["windows", "linux"])
                run = self.gobco(calls)[0]
                self.assertEqual(run["goos"], "windows")
                self.assertEqual(run["files"], ["main.go", "main_test.go", "proc_windows.go", "proc_windows_test.go"])
                said = proc.stdout + proc.stderr
                if status:
                    self.assertIn("GOBCO_GATE=beyond:linux/amd64: 1 condition(s) in %s were not evaluated both ways:"
                                  % held, proc.stderr)
                else:
                    self.assertIn("GOBCO_GATE=beyond:linux/amd64: every condition in %s was evaluated both ways"
                                  % held, proc.stdout)
                self.assertEqual("%d condition(s) in files linux/amd64 also builds were not, and are left to a run there"
                                 % left in said, left > 0 and not status, said)
                self.assertEqual(self.staged_copies(), [])

    def test_gate_beyond_on_the_reference_itself_holds_nothing(self):
        proc, _ = self.run_script("./cmd/tool", env={
            "GOBCO_GATE": "beyond:linux/amd64",
            "STUB_GOBCO_CONDITIONS": 'main.go:3:9: condition "x" was never evaluated'})
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assertIn("GOBCO_GATE=beyond:linux/amd64: linux/amd64 under build tags (none) builds no source of "
                      "./cmd/tool that linux/amd64 does not, so the gate holds nothing here; 1 condition(s) in "
                      "files linux/amd64 also builds were not, and are left to a run there", proc.stdout)

    def test_gate_beyond_a_package_the_reference_builds_nothing_of_holds_every_source(self):
        # The go command answers the reference's listing with an error rather
        # than an empty package, and that error is the answer: every source.
        proc, _ = self.run_script("./internal/winonly", env={
            "GOOS": "windows", "GOBCO_GATE": "beyond:linux/amd64",
            "STUB_GOBCO_CONDITIONS": 'w_windows.go:4:9: condition "c == 1" was never evaluated'})
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertIn("builds and linux/amd64 does not (w_windows.go)", proc.stderr)

    def test_gate_reference_go_cannot_load_is_refused(self):
        proc, calls = self.run_script("./cmd/tool", env={
            "GOOS": "windows", "GOBCO_GATE": "beyond:linux/amd64", "STUB_LIST_ERROR_FOR_GOOS": "linux"})
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertEqual(self.gobco(calls), [])
        self.assertIn("go cannot load ./cmd/tool for linux/amd64 under build tags (none), which "
                      "GOBCO_GATE=beyond:linux/amd64 compares this build with: planted load error for linux",
                      proc.stderr)

    def test_gate_a_value_it_cannot_read_is_refused_before_anything_runs(self):
        for value in ("some", "beyond:linux", "beyond:", "beyond:linux/amd64/v3", "Beyond:linux/amd64", "ALL"):
            with self.subTest(value):
                proc, calls = self.run_script("./internal/plain", env={"GOBCO_GATE": value})
                self.assertEqual(proc.returncode, 2, proc.stdout + proc.stderr)
                self.assertEqual(calls, [])
                self.assertIn("GOBCO_GATE=%s is neither all nor beyond:GOOS/GOARCH" % value, proc.stderr)

    def test_test_flags_reach_gobcos_go_test_one_per_word(self):
        cases = [
            ("no tags", (), ["run", GOBCO, "-test=-timeout=30m", "-test=-v"]),
            ("tags", ("e2e",), ["run", GOBCO, "-test=-tags=e2e", "-test=-timeout=30m", "-test=-v"]),
        ]
        for name, extra, argv in cases:
            with self.subTest(name):
                proc, calls = self.run_script("./internal/plain", *extra,
                                              env={"GOBCO_TEST_FLAGS": " -timeout=30m  -v "})
                self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
                self.assertEqual(self.gobco(calls)[0]["argv"], argv)

    def test_cross_platform_legs_run_the_gate_through_bash(self):
        # The Windows and macOS legs run the script itself through bash, not
        # make, which windows-latest does not promise, on the two packages
        # whose files split by platform, gated beyond the platform the local
        # gate is applied on, with the timeout cmd/server needs on Windows.
        with open(CI_WORKFLOW, encoding="utf-8") as fh:
            text = fh.read()
        step = re.search(r"^      - name: Condition coverage of the platform halves\n((?:        .*\n|\n)+)",
                         text, re.MULTILINE)
        self.assertIsNotNone(step, "the cross-platform gobco step is missing")
        body = step.group(1)
        self.assertIn("if: matrix.os != 'ubuntu-latest'", body)
        self.assertIn("shell: bash", body)
        self.assertIn("GOBCO_GATE: beyond:linux/amd64", body)
        self.assertIn("GOBCO_TEST_FLAGS: -timeout=30m", body)
        for pkg in ("./cmd/server", "./internal/toolutil"):
            self.assertIn(pkg, body)
        self.assertIn("bash scripts/coverage-conditions.sh", body)
        self.assertNotIn("make ", body)

    def test_an_interrupt_ends_the_run_and_removes_the_copy(self):
        # A handler that only cleaned up would return to the script, which
        # would carry on against the copy it had just removed. A signal sent
        # to the script alone is the case that tells the two apart: gobco
        # finishes normally, and a returning handler would let the script pass
        # its report on and exit 0. SIGINT to the whole process group, as a
        # terminal sends it, is held too.
        cases = [
            ("SIGTERM to the script", signal.SIGTERM, False, 143),
            ("SIGINT to the script", signal.SIGINT, False, 130),
            ("SIGINT to the process group", signal.SIGINT, True, 130),
        ]
        for name, signum, group, status in cases:
            with self.subTest(name):
                open(self.log, "w", encoding="utf-8").close()
                started = os.path.join(self.scratch, "started")
                if os.path.exists(started):
                    os.remove(started)
                proc = subprocess.Popen(
                    [SCRIPT, "./cmd/tool"], cwd=self.root, start_new_session=True,
                    env=self.environment({"STUB_GOBCO_SLEEP": "1", "STUB_STARTED": started}),
                    stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
                deadline = time.monotonic() + 30
                while not os.path.exists(started) and time.monotonic() < deadline:
                    time.sleep(0.02)
                self.assertTrue(os.path.exists(started), "gobco never started")
                staged = [entry for entry in self.staged_copies()
                          if os.path.isdir(os.path.join(self.tmp, entry, "module", "cmd", "tool"))]
                self.assertEqual(len(staged), 1, self.staged_copies())
                if group:
                    os.killpg(proc.pid, signum)
                else:
                    proc.send_signal(signum)
                out, err = proc.communicate(timeout=30)
                self.assertEqual(proc.returncode, status, out + err)
                self.assertEqual(self.staged_copies(), [])

    def test_makefile_target_runs_the_script_with_the_tags(self):
        with open(MAKEFILE, encoding="utf-8") as fh:
            text = fh.read()
        recipe = re.search(r"^coverage-conditions:\n((?:\t.*\n)+)", text, re.MULTILINE)
        self.assertIsNotNone(recipe, "the coverage-conditions target is missing")
        self.assertIn("scripts/coverage-conditions.sh $(PKG) $(TAGS)", recipe.group(1))
        self.assertNotIn("rillig/gobco", recipe.group(1))

    def test_gobco_is_run_through_the_script_everywhere(self):
        # One way to run gobco, so a package in the shape of issue 1017 is
        # staged by the gate over action specs too rather than reported as
        # not measured, and one pin of its version.
        with open(SPEC_CONDITIONS, encoding="utf-8") as fh:
            spec = fh.read()
        self.assertIn("scripts/coverage-conditions.sh", spec)
        self.assertNotIn("rillig/gobco", spec)
        with open(SCRIPT, encoding="utf-8") as fh:
            self.assertIn(GOBCO, fh.read())

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
