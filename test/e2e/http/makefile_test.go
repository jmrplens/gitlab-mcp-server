// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build httpe2e

// makefile_test.go runs the Makefile's recipes where they hand the shell a
// path nobody in the recipe chose, against stand-ins for the tools they
// start: a tool's path, which the environment decides (PATH, GOBIN, GOPATH or
// HOME), and a directory a make variable names, which an override can give a
// space. Each is held to reaching the shell as one path.
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
// install into.
const (
	standInRecordsItself    = "#!/bin/sh\nprintf '%s\\n' \"$0\" > \"$FAKE_STATE/ran\"\n"
	standInRecordsGotestsum = "#!/bin/sh\nprintf '%s\\n' \"$GOTESTSUM\" > \"$FAKE_STATE/ran\"\n"
	standInRecordsGOBIN     = "#!/bin/sh\nprintf '%s\\n' \"$GOBIN\" > \"$FAKE_STATE/ran\"\n"
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
// The directory is created by the Makefile's MKDIR_P, which quotes the one
// path it takes, and the report was written through redirections that did
// not: the first echo wrote "dist/analysis", a file named by the first word,
// and every section after it was appended there, leaving no report where the
// target says it saved one.
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
// e2e-go-coverage with a coverage root whose path holds a space, runs it in a
// tree holding two leaves under that root with a stand-in go, and holds it to
// merging both, each as one path.
//
// The recipe listed the meta-data files with find over the root unquoted and
// split find's answer at every space, so it searched dist/e2e and cover
// rather than the root and refused the run as one that left no coverage data.
func TestE2EGoCoverageTarget_CoverRootWithASpace_MergesEveryLeaf(t *testing.T) {
	t.Parallel()
	makeBin := makeOnLinux(t)
	rendered := renderMake(t, makeBin, repoRoot(), "e2e-go-coverage", "E2E_COVER_ROOT=dist/e2e cover", "E2E_REPORT_DIR="+t.TempDir())
	start := strings.Index(rendered, "bash -o pipefail -c '")
	if start < 0 {
		t.Fatalf("make -n printed no recipe merging the counters under bash:\n%s", rendered)
	}
	tree := t.TempDir()
	touch(t,
		filepath.Join(tree, "dist", "e2e cover", "stdio", "covmeta.0"),
		filepath.Join(tree, "dist", "e2e cover", "http", "covmeta.0"))
	tools := t.TempDir()
	record := filepath.Join(tools, "go-args")
	// One argument a line, and a blank line after each call.
	writeStandIn(t, filepath.Join(tools, "go"), "#!/bin/sh\nprintf '%s\\n' \"$@\" '' >> \"$FAKE_RECORD\"\n")

	//#nosec G204 -- the recipe is what make rendered from this repository's own Makefile
	merge := exec.CommandContext(t.Context(), "sh", "-c", rendered[start:])
	merge.Dir = tree
	merge.Env = append(os.Environ(), "FAKE_RECORD="+record, "PATH="+tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := merge.CombinedOutput(); err != nil {
		t.Fatalf("the merge ended with %v, want it to merge both leaves:\n%s", err, out)
	}
	calls, err := os.ReadFile(record) //#nosec G304 -- the record the stand-in wrote in this test's own directory
	if err != nil {
		t.Fatalf("the merge never ran go: %v", err)
	}
	want := "\n-i=dist/e2e cover/http,dist/e2e cover/stdio\n"
	if got := strings.Count(string(calls), want); got != 2 {
		t.Errorf("go was handed %q %d times, want twice, to textfmt and to percent; it was called with:\n%s", strings.Trim(want, "\n"), got, calls)
	}
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
