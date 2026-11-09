// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build httpe2e

// makefile_test.go runs the Makefile's recipes where they hand the shell a
// path nobody in the recipe chose, against stand-ins for the tools they
// start: a tool's path, which the environment decides (PATH, GOBIN, GOPATH or
// HOME), and a directory or a file a make variable names, which an override
// can give a space. Each is held to reaching the tool as one path.
//
// They sit beside the Cloudflare Tunnel target's own cases in
// cloudflare_tunnel_test.go, which render and run that target the same way,
// and run where those do: on Linux, in CI's e2e-http job.
package httpe2e

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// The stand-ins these tests put in place of the tools a recipe starts. Each
// one that records writes what it was handed into $FAKE_STATE/ran: the path
// it was started by, the GOTESTSUM it was handed, or the GOBIN it would
// install into. standInRecordsArgs appends its arguments to $FAKE_STATE/args
// instead, one a line and a blank line after each call, so a path the shell
// split reads as two lines.
const (
	standInRecordsItself    = "#!/bin/sh\nprintf '%s\\n' \"$0\" > \"$FAKE_STATE/ran\"\n"
	standInRecordsGotestsum = "#!/bin/sh\nprintf '%s\\n' \"$GOTESTSUM\" > \"$FAKE_STATE/ran\"\n"
	standInRecordsGOBIN     = "#!/bin/sh\nprintf '%s\\n' \"$GOBIN\" > \"$FAKE_STATE/ran\"\n"
	standInRecordsArgs      = "#!/bin/sh\nprintf '%s\\n' \"$@\" '' >> \"$FAKE_STATE/args\"\n"
	standInSilent           = "#!/bin/sh\nexit 0\n"
)

// TestMakeRunTargets_ToolUnderAPathWithASpace_RunAsOnePath runs each target
// that runs gotestsum, pkgsite or the MCP Registry publisher, or hands one on
// to a script, in a checkout whose path holds a space, with the tool under a
// folder whose path holds one too, and holds the target to running the tool,
// or handing it on, as one path.
//
// The tool's path is the environment's: gotestsum's comes from the PATH,
// GOBIN or GOPATH, pkgsite's from the PATH or GOPATH, and the publisher's
// from HOME. The recipes expanded it unquoted, so the shell split it at the
// space and each of these targets ended with status 127 before the tool
// started, while ensure-gotestsum, which quotes the same path, had found or
// installed it there. The tunnel target's own run of gotestsum is held by
// the tests in cloudflare_tunnel_test.go, whose stand-ins sit under such a
// folder too.
func TestMakeRunTargets_ToolUnderAPathWithASpace_RunAsOnePath(t *testing.T) {
	t.Parallel()
	makeBin := makeOnLinux(t)
	checkout := checkoutWithASpace(t, ".github/workflows/release.yml")
	tools := filepath.Join(t.TempDir(), "tools with space")
	gotestsum := filepath.Join(tools, "gotestsum")
	pkgsite := filepath.Join(tools, "pkgsite")
	writeStandIn(t, gotestsum, standInRecordsItself)
	writeStandIn(t, pkgsite, standInRecordsItself)
	writeStandIn(t, filepath.Join(tools, "go"), standInRecordsGOBIN)
	writeStandIn(t, filepath.Join(checkout, "test", "e2e", "scripts", "run-docker-e2e.sh"), standInRecordsGotestsum)

	// The publisher sits under HOME, in a folder named after the version the
	// Makefile pins: one home holds it and the other does not, so the second
	// is where ensure-mcp-publisher installs it.
	version := strings.TrimSpace(runMake(t, makeBin, checkout, nil, "mcp-publisher-version"))
	publisherDir := func(home string) string {
		return filepath.Join(home, ".cache", "gitlab-mcp-server", "mcp-publisher", version)
	}
	installed := filepath.Join(t.TempDir(), "home dir")
	writeStandIn(t, filepath.Join(publisherDir(installed), "publisher"), standInRecordsItself)
	bare := filepath.Join(t.TempDir(), "home dir")

	tests := []struct {
		goal string
		// home is the HOME the target runs under, bare when empty.
		home string
		args []string
		// want is what the stand-in recorded: the tool's whole path.
		want string
	}{
		{goal: "test-e2e-harness", want: gotestsum},
		{goal: "test-e2e-stdio", want: gotestsum},
		{goal: "test-e2e-http", want: gotestsum},
		{goal: "test-e2e-collector", want: gotestsum},
		{goal: "test-e2e-gitlab", want: gotestsum},
		{goal: "test-e2e-ce", want: gotestsum},
		{goal: "test-e2e-ee", want: gotestsum},
		{goal: "modeleval-ce", want: gotestsum},
		{goal: "modeleval-ee", want: gotestsum},
		{goal: "docs-local-go", want: pkgsite},
		{goal: "check-server-json", home: installed, args: []string{"-o", "ensure-mcp-publisher"}, want: filepath.Join(publisherDir(installed), "publisher")},
		{goal: "ensure-mcp-publisher", want: publisherDir(bare)},
	}
	for _, tt := range tests {
		t.Run(tt.goal, func(t *testing.T) {
			t.Parallel()
			state := t.TempDir()
			home := tt.home
			if home == "" {
				home = bare
			}
			args := append([]string{
				"-o", "ensure-gotestsum", "-o", "e2e-server-binary", tt.goal,
				"GOTESTSUM=" + gotestsum, "PKGSITE=" + pkgsite,
			}, tt.args...)
			runMake(t, makeBin, checkout, []string{
				"HOME=" + home, "GOBIN=", "FAKE_STATE=" + state,
				"PATH=" + tools + string(os.PathListSeparator) + os.Getenv("PATH"),
			}, args...)
			got, err := os.ReadFile(filepath.Join(state, "ran")) //#nosec G304 -- the mark a stand-in wrote in this test's own directory
			if err != nil {
				t.Fatalf("make %s ran no stand-in: %v", tt.goal, err)
			}
			if string(got) != tt.want+"\n" {
				t.Errorf("make %s ran or handed on %q, want %q", tt.goal, strings.TrimSuffix(string(got), "\n"), tt.want)
			}
		})
	}
}

// TestAnalyzeReportTarget_AnalysisDirWithASpace_WritesTheReportThere runs
// make analyze-report with an ANALYSIS_DIR whose path holds a space, against
// stand-ins for the three analysers, and holds it to writing its report into
// that directory.
//
// It holds what quoting MKDIR_P would otherwise have broken. With the path
// unquoted, the target failed outright, since mkdir made dist/analysis a
// directory and the first redirection could not write to it. With MKDIR_P
// quoted and the redirections not, the target would make "dist/analysis x"
// and write the whole report to a file named by the path's first word,
// dist/analysis, leaving none where it says it saved one.
func TestAnalyzeReportTarget_AnalysisDirWithASpace_WritesTheReportThere(t *testing.T) {
	t.Parallel()
	makeBin := makeOnLinux(t)
	checkout := checkoutWithASpace(t)
	tools := t.TempDir()
	writeStandIn(t, filepath.Join(tools, "golangci-lint"), standInSilent)
	writeStandIn(t, filepath.Join(tools, "govulncheck"), standInSilent)
	writeStandIn(t, filepath.Join(tools, "npx"), standInSilent)
	runMake(t, makeBin, checkout, []string{
		"PATH=" + tools + string(os.PathListSeparator) + os.Getenv("PATH"),
	}, "analyze-report", "ANALYSIS_DIR=dist/analysis x")
	report, err := os.ReadFile(filepath.Join(checkout, "dist", "analysis x", "report.txt")) //#nosec G304 -- the report the target wrote in this test's own checkout
	if err != nil {
		t.Fatalf("make analyze-report wrote no report where ANALYSIS_DIR says: %v", err)
	}
	if want := "# Static Analysis Report - gitlab-mcp-server\n"; !strings.HasPrefix(string(report), want) {
		t.Errorf("the report opens with %q, want %q", strings.SplitAfter(string(report), "\n")[0], want)
	}
}

// TestE2EGoCoverageTarget_CoverRootWithASpace_MergesEveryLeaf renders make
// e2e-go-coverage with a coverage root and a report directory whose paths
// hold a space, runs it in a tree holding two leaves under that root with a
// stand-in go, and holds it to merging both, each as one path, into a profile
// under that report directory.
//
// The recipe listed the meta-data files with find over the root unquoted and
// split find's answer at every space, so it searched dist/e2e and cover
// rather than the root and refused the run as one that left no coverage data.
// It handed the merged profile's path to textfmt and to cover unquoted as
// well, so both read a file named by the report directory's first word.
func TestE2EGoCoverageTarget_CoverRootWithASpace_MergesEveryLeaf(t *testing.T) {
	t.Parallel()
	makeBin := makeOnLinux(t)
	rendered := renderMake(t, makeBin, repoRoot(), "e2e-go-coverage", "E2E_COVER_ROOT=dist/e2e cover", "E2E_REPORT_DIR=dist/e2e reports")
	start := strings.Index(rendered, "bash -o pipefail -c '")
	if start < 0 {
		t.Fatalf("make -n printed no recipe merging the counters under bash:\n%s", rendered)
	}
	tree := t.TempDir()
	touch(t,
		filepath.Join(tree, "dist", "e2e cover", "stdio", "covmeta.0"),
		filepath.Join(tree, "dist", "e2e cover", "http", "covmeta.0"))
	tools, state := t.TempDir(), t.TempDir()
	writeStandIn(t, filepath.Join(tools, "go"), standInRecordsArgs)

	//#nosec G204 -- the recipe is what make rendered from this repository's own Makefile
	merge := exec.CommandContext(t.Context(), "sh", "-c", rendered[start:])
	merge.Dir = tree
	merge.Env = append(os.Environ(), "FAKE_STATE="+state, "PATH="+tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := merge.CombinedOutput(); err != nil {
		t.Fatalf("the merge ended with %v, want it to merge both leaves:\n%s", err, out)
	}
	calls := recordedArgs(t, state)
	want := "\n-i=dist/e2e cover/http,dist/e2e cover/stdio\n"
	if got := strings.Count(calls, want); got != 2 {
		t.Errorf("go was handed %q %d times, want twice, to textfmt and to percent; it was called with:\n%s", strings.Trim(want, "\n"), got, calls)
	}
	assertCalled(t, "make e2e-go-coverage", calls,
		[]string{"tool", "covdata", "textfmt", "-i=dist/e2e cover/http,dist/e2e cover/stdio", "-o=dist/e2e reports/e2e-go-coverage.out"},
		[]string{"tool", "cover", "-func=dist/e2e reports/e2e-go-coverage.out"})
}

// TestMakeTargets_PathWithASpace_ReachesTheToolWhole runs each target that
// hands a tool a path under a directory a make variable names, or a file one
// names, with that variable holding a space, in a checkout whose path holds
// one too, against stand-ins for the tools, and holds the target to handing
// every such path on as one argument.
//
// The recipes expanded those paths unquoted, and make's wildcard and abspath
// split their argument the same way, so the tools were handed pieces. Some
// targets then failed, and some did not: given a report directory of
// "dist/e2e reports", which MKDIR_P makes whole, audit-e2e-coverage wrote its
// report to a file named dist/e2e and exited 0 without the summary it was
// asked for; the model results targets read the shards from the path's first
// word and dropped -refold and -render; e2e-support-coverage merged the first
// word as a run profile.
func TestMakeTargets_PathWithASpace_ReachesTheToolWhole(t *testing.T) {
	t.Parallel()
	makeBin := makeOnLinux(t)
	checkout := checkoutWithASpace(t)
	tools := filepath.Join(t.TempDir(), "tools")
	writeStandIns(t, tools, standInRecordsArgs, "go", "gotestsum", "rsvg-convert", "cwebp", "docker")
	// brand-rasters' scratch file, under a TMPDIR whose path holds a space.
	raster := filepath.Join(t.TempDir(), "tmp dir", "raster")
	writeStandIn(t, filepath.Join(tools, "mktemp"), "#!/bin/sh\nprintf '%s\\n' \"$FAKE_TMP\"\n")

	const (
		reports  = "dist/e2e reports"
		calls    = "dist/e2e calls"
		record   = "docs/e2e support.json"
		shards   = "dist/modeleval shards/ce"
		binaries = "dist/release binaries"
		offline  = reports + "/e2e-support-offline.out"
	)
	// The run profile e2e-support-coverage merges, the calls directory
	// audit-1to1-grants-report finds, and the binaries the validate targets
	// mount.
	touch(t,
		filepath.Join(checkout, filepath.FromSlash(reports), "e2e-ce-coverprofile.out"),
		filepath.Join(checkout, filepath.FromSlash(calls), "shard"),
		filepath.Join(checkout, filepath.FromSlash(binaries), "gitlab-mcp-server-linux-amd64"))
	mounted := filepath.Join(checkout, filepath.FromSlash(binaries)) + ":/binaries:ro"

	tests := []struct {
		goal string
		args []string
		// want holds the calls the stand-ins were to be handed, each the
		// arguments it holds in a row.
		want [][]string
	}{
		{goal: "test-e2e-harness", want: [][]string{{"--junitfile", reports + "/e2e-harness-junit.xml"}}},
		{goal: "test-e2e-stdio", want: [][]string{{"--junitfile", reports + "/e2e-stdio-junit.xml"}}},
		{goal: "test-e2e-http", want: [][]string{{"--junitfile", reports + "/e2e-http-junit.xml"}}},
		{goal: "test-e2e-collector", want: [][]string{{"--junitfile", reports + "/e2e-collector-junit.xml"}}},
		{goal: "test-e2e-gitlab", want: [][]string{{"--junitfile", reports + "/e2e-gitlab-junit.xml", "--jsonfile", reports + "/e2e-gitlab-log.json"}}},
		{
			goal: "e2e-server-binary", args: []string{"E2E_SERVER_BINARY=dist/e2e server/gitlab-mcp-server"},
			want: [][]string{{"-o", "dist/e2e server/gitlab-mcp-server", "./cmd/server"}},
		},
		{goal: "audit-e2e-gaps", want: [][]string{{"-calls", calls, "-report", ""}}},
		{goal: "audit-e2e-coverage", want: [][]string{{"-calls", calls, "-report", "-o", reports + "/e2e-coverage.json", "-summary", "-"}}},
		{
			goal: "e2e-coverage-record-ce",
			want: [][]string{{"-calls", calls + "/ce", "-results", reports + "/e2e-ce-log.json", "-runtime", "ce", "-static", "-record", "-o", reports + "/e2e-coverage-ce.json"}},
		},
		{
			goal: "e2e-coverage-record-ee",
			want: [][]string{{"-calls", calls + "/ee", "-results", reports + "/e2e-ee-log.json", "-runtime", "ee", "-static", "-record", "-o", reports + "/e2e-coverage-ee.json"}},
		},
		{goal: "audit-1to1-paths-e2e", want: [][]string{{"-e2e-calls", calls, "-output"}}},
		{goal: "audit-1to1-grants-report", want: [][]string{{"-scope=grants", "-e2e-calls", calls, "-output"}}},
		{
			goal: "e2e-support-coverage",
			want: [][]string{
				{"-coverprofile=" + offline, "./test/e2e/internal/fixture"},
				{"-offline", offline, "-online", reports + "/e2e-ce-coverprofile.out", "-o", reports + "/e2e-support-coverage.out", ""},
			},
		},
		{
			goal: "e2e-support-coverage-record", args: []string{"E2E_SUPPORT_RECORD=" + record},
			want: [][]string{{"-o", reports + "/e2e-support-coverage.out", "-write-record", record, ""}},
		},
		{
			goal: "check-e2e-support-coverage", args: []string{"E2E_SUPPORT_RECORD=" + record},
			want: [][]string{
				{"-coverprofile=" + offline, "./test/e2e/internal/fixture"},
				{"-offline", offline, "-check-record", record, "-freshness"},
			},
		},
		{goal: "model-results-record", args: []string{"MODELEVAL_SHARDS=" + shards}, want: [][]string{{"-shards", shards, "-render", ""}}},
		{goal: "model-results-refold", args: []string{"MODELEVAL_SHARDS=" + shards}, want: [][]string{{"-shards", shards, "-refold", "-render", ""}}},
		{goal: "model-results-dry-run", args: []string{"MODELEVAL_SHARDS=" + shards}, want: [][]string{{"-shards", shards, "-dry-run", "-render", ""}}},
		{goal: "validate-npm", args: []string{"NPM_BINARIES=" + binaries}, want: [][]string{{"-v", mounted}}},
		{goal: "validate-pypi", args: []string{"PYPI_BINARIES=" + binaries}, want: [][]string{{"-v", mounted}}},
		{goal: "validate-nuget", args: []string{"NUGET_BINARIES=" + binaries}, want: [][]string{{"-v", mounted}}},
		{
			goal: "brand-rasters",
			want: [][]string{{".github/brand/banner.svg", "-o", raster, ""}, {"-quiet", "-lossless", raster, "-o", ".github/brand/banner.webp", ""}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.goal, func(t *testing.T) {
			t.Parallel()
			state := t.TempDir()
			args := []string{"-o", "ensure-gotestsum", "-o", "brand", tt.goal, "E2E_REPORT_DIR=" + reports, "E2E_CALLS_DIR=" + calls, "GOTESTSUM=" + filepath.Join(tools, "gotestsum")}
			if tt.goal != "e2e-server-binary" {
				args = append(args, "-o", "e2e-server-binary")
			}
			runMake(t, makeBin, checkout, []string{
				"FAKE_STATE=" + state, "FAKE_TMP=" + raster,
				"PATH=" + tools + string(os.PathListSeparator) + os.Getenv("PATH"),
			}, append(args, tt.args...)...)
			assertCalled(t, "make "+tt.goal, recordedArgs(t, state), tt.want...)
		})
	}
}

// TestInspectorTargets_BinaryPathWithASpace_RemoveThatFileAlone runs make
// inspector with an INSPECTOR_BIN whose path holds a space, against stand-ins
// for go and npx, then the removal make inspector-stop renders, and holds
// both to building, starting and removing that one file, and no file named by
// the path's first word.
//
// The recipes expanded the path unquoted, so the build wrote to the first
// word and the removal on exit deleted whatever file the first word named.
// make inspector-stop is rendered rather than run, since it stops every
// Inspector process on the machine.
func TestInspectorTargets_BinaryPathWithASpace_RemoveThatFileAlone(t *testing.T) {
	t.Parallel()
	makeBin := makeOnLinux(t)
	checkout := checkoutWithASpace(t)
	if err := os.WriteFile(filepath.Join(checkout, ".env"), []byte("GITLAB_TOKEN=stand-in\n"), 0o600); err != nil {
		t.Fatalf("writing the checkout's .env: %v", err)
	}
	tools, state := t.TempDir(), t.TempDir()
	writeStandIns(t, tools, standInRecordsArgs, "go", "npx")
	dir := t.TempDir()
	decoy, binary := filepath.Join(dir, "inspector"), filepath.Join(dir, "inspector bin")
	touch(t, decoy, binary)

	runMake(t, makeBin, checkout, []string{
		"FAKE_STATE=" + state, "PATH=" + tools + string(os.PathListSeparator) + os.Getenv("PATH"),
	}, "inspector", "INSPECTOR_BIN="+binary)
	assertCalled(t, "make inspector", recordedArgs(t, state),
		[]string{"-o", binary, "./cmd/server", ""}, []string{"--", binary, ""})
	assertRemovedAlone(t, binary, decoy)

	touch(t, binary)
	rendered := renderMake(t, makeBin, checkout, "inspector-stop", "INSPECTOR_BIN="+binary)
	var removal string
	for line := range strings.Lines(rendered) {
		if strings.HasPrefix(line, "rm -f ") {
			removal = line
		}
	}
	if removal == "" {
		t.Fatalf("make -n inspector-stop printed no removal:\n%s", rendered)
	}
	//#nosec G204 -- a line make rendered from this repository's own Makefile
	remove := exec.CommandContext(t.Context(), "sh", "-c", removal)
	remove.Dir = t.TempDir()
	if out, err := remove.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v: %s", strings.TrimSpace(removal), err, out)
	}
	assertRemovedAlone(t, binary, decoy)
}

// assertRemovedAlone fails the test unless removed is gone and kept is there.
func assertRemovedAlone(t *testing.T, removed, kept string) {
	t.Helper()
	if _, err := os.Stat(removed); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%s is still there (%v), want it removed", removed, err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("%s, named by the first word of the path, was removed: %v", kept, err)
	}
}

// assertCalled fails the test for each of calls that got, a record
// recordedArgs returned, does not hold in a row, naming what handed it on.
func assertCalled(t *testing.T, what, got string, calls ...[]string) {
	t.Helper()
	for _, call := range calls {
		if !strings.Contains(got, argLines(call)) {
			t.Errorf("%s handed on no call holding %q; the stand-ins were called with:\n%s", what, call, got)
		}
	}
}

// recordedArgs returns what standInRecordsArgs stand-ins appended under
// state, with a newline before it, so argLines matches a call's first
// argument as it matches any other.
func recordedArgs(t *testing.T, state string) string {
	t.Helper()
	calls, err := os.ReadFile(filepath.Join(state, "args")) //#nosec G304 -- the record the stand-ins wrote in this test's own directory
	if err != nil {
		t.Fatalf("no stand-in was called: %v", err)
	}
	return "\n" + string(calls)
}

// argLines returns args as recordedArgs holds them in a row: each on a line of
// its own, an empty one standing for the end of the call.
func argLines(args []string) string {
	return "\n" + strings.Join(args, "\n") + "\n"
}

// makeOnLinux returns make's path, skipping the test where make, sh or bash
// is missing or the platform is not Linux. CI's e2e-http job runs this module
// on Linux; macOS ships make 3.81, older than the 4.2 whose $(file <) the
// Makefile reads its version with, and on Windows the targets run under WSL.
func makeOnLinux(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skipf("the Makefile's recipes are held on Linux, where CI's e2e-http job runs them; this is %s", runtime.GOOS)
	}
	makeBin, err := exec.LookPath("make")
	if err != nil {
		t.Skipf("make is not installed here, so the recipe cannot be rendered: %v", err)
	}
	for _, tool := range []string{"sh", "bash"} {
		if _, err = exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed here, so the rendered recipe cannot be run: %v", tool, err)
		}
	}
	return makeBin
}

// makeEnviron returns this process's environment for a make the test starts,
// followed by extra: without CI, which the tunnel target refuses at parse
// time, and without what an enclosing make or a COVER=1 run would hand it.
func makeEnviron(extra ...string) []string {
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		return slices.Contains([]string{"CI", "COVER", "E2E_COVER_ROOT", "MAKEFLAGS", "MFLAGS", "MAKELEVEL"}, name)
	})
	return append(env, extra...)
}

// renderMake returns everything make -n prints for args in dir, the
// prerequisites' commands included.
func renderMake(t *testing.T, makeBin, dir string, args ...string) string {
	t.Helper()
	return runMake(t, makeBin, dir, nil, append([]string{"-n"}, args...)...)
}

// runMake runs make with args in dir, under makeEnviron(extra...), and
// returns what it printed, failing the test with its output when it ends with
// a status other than 0.
func runMake(t *testing.T, makeBin, dir string, extra []string, args ...string) string {
	t.Helper()
	args = append([]string{"-s", "--no-print-directory"}, args...)
	//#nosec G204 -- make from the PATH, with the test's own arguments
	cmd := exec.CommandContext(t.Context(), makeBin, args...)
	cmd.Dir = dir
	cmd.Env = makeEnviron(extra...)
	out, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			stderr = exit.Stderr
		}
		t.Fatalf("make %s: %v:\n%s%s", strings.Join(args, " "), err, out, stderr)
	}
	return string(out)
}

// checkoutWithASpace copies the Makefile, the two files make reads while it
// parses it (it includes nothing) and the files named, into a directory of
// the test's own whose path holds a space, and returns that directory.
func checkoutWithASpace(t *testing.T, names ...string) string {
	t.Helper()
	checkout := filepath.Join(t.TempDir(), "checkout with space")
	var copyErr error
	for _, name := range append([]string{"Makefile", "VERSION", "go.mod"}, names...) {
		path := filepath.Join(checkout, filepath.FromSlash(name))
		body, err := os.ReadFile(filepath.Join(repoRoot(), filepath.FromSlash(name))) //#nosec G304 -- a file of this repository, named by the test
		if err == nil {
			err = os.MkdirAll(filepath.Dir(path), 0o750)
		}
		if err == nil {
			err = os.WriteFile(path, body, 0o600) //#nosec G703 -- a t.TempDir and a name the test gave
		}
		copyErr = errors.Join(copyErr, err)
	}
	if copyErr != nil {
		t.Fatalf("copying the Makefile and what it reads into %s: %v", checkout, copyErr)
	}
	return checkout
}

// touch creates each of paths as an empty file, with its folder.
func touch(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("creating the folder of %s: %v", path, err)
		}
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
	}
}

// writeStandIns writes body as an executable stand-in in dir under each of
// names.
func writeStandIns(t *testing.T, dir, body string, names ...string) {
	t.Helper()
	for _, name := range names {
		writeStandIn(t, filepath.Join(dir, name), body)
	}
}

// writeStandIn writes an executable stand-in at path, creating its folder.
func writeStandIn(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("creating the folder of the stand-in %s: %v", path, err)
	}
	//#nosec G306 -- a stand-in a recipe must execute
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatalf("writing the stand-in %s: %v", path, err)
	}
}
