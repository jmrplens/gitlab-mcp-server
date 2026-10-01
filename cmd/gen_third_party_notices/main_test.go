// main_test.go drives the command from its arguments: the exit code each
// kind of failure gets, the file a successful run writes, and one run against
// a real binary with the real `go env`, which is the path the release takes.
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

// TestRunMain_ExitCodes holds the exit code to what went wrong: 2 for
// arguments that do not parse or name no binary, 0 for -h, 1 for notices that
// could not be generated.
func TestRunMain_ExitCodes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		args   []string
		code   int
		stderr string
	}{
		{"help", []string{"-h"}, 0, "-targets"},
		{"an unknown flag", []string{"-nope"}, 2, "flag provided but not defined"},
		{"no binary named", []string{"-o", "x"}, 2, "name at least one binary"},
		{"a binary that is not there", []string{filepath.Join(t.TempDir(), "absent")}, 1, "matches no file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			if code := runMain(tc.args, &stdout, &stderr); code != tc.code {
				t.Fatalf("exit %d, want %d\nstderr:\n%s", code, tc.code, stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.stderr) {
				t.Errorf("stderr lacks %q:\n%s", tc.stderr, stderr.String())
			}
		})
	}
}

// TestRunMain_ReadsARealBinary runs the command the way the release does:
// against a real binary, its build information read by debug/buildinfo and
// GOROOT and the module cache by `go env`. The binary is this test's own,
// which links no module outside this one, so its notices are the standard
// library's, read from the GOROOT of the toolchain that built it.
func TestRunMain_ReadsARealBinary(t *testing.T) {
	t.Parallel()

	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		t.Fatal("the test binary records no build information")
	}
	target, err := targetOf(info)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "THIRD_PARTY_NOTICES")
	var stdout, stderr bytes.Buffer
	if code := runMain([]string{"-o", out, "-targets", target, self}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d\nstderr:\n%s", code, stderr.String())
	}
	doc, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	toolchain, _, _ := strings.Cut(info.GoVersion, " ")
	for _, want := range []string{"Builds:    " + target + "\n", "Go standard library " + toolchain + "\n", "--- LICENSE ---"} {
		t.Run(want, func(t *testing.T) {
			t.Parallel()
			if !strings.Contains(string(doc), want) {
				t.Errorf("the notices lack %q:\n%s", want, doc)
			}
		})
	}
	if !strings.Contains(stdout.String(), "and the Go standard library for "+target) {
		t.Errorf("the summary does not name the target:\n%s", stdout.String())
	}
}

// TestRun_WritesTheNoticesAndSaysWhat covers a successful run end to end
// over the fixture release, and each later step that can stop it: the
// target list, `go env` and the write itself.
func TestRun_WritesTheNoticesAndSaysWhat(t *testing.T) {
	t.Parallel()

	dir, read := releaseFixture(t)
	goroot := fixtureGOROOT(t, fixtureGo)
	modcache := fixtureModcache(t)
	base := func(out string) config {
		return config{
			patterns: []string{filepath.Join(dir, "server-*")},
			out:      out,
			targets:  []string{"linux/amd64", "windows/amd64"},
			goroot:   goroot,
			modcache: modcache,
			readInfo: read,
			goEnv:    func() ([]byte, error) { t.Error("go env asked although both were named"); return nil, nil },
		}
	}

	out := filepath.Join(t.TempDir(), "THIRD_PARTY_NOTICES")
	summary, err := run(base(out))
	if err != nil {
		t.Fatal(err)
	}
	if want := "gen_third_party_notices: " + out + " covers 4 modules and the Go standard library for linux/amd64, windows/amd64"; summary != want {
		t.Errorf("summary = %q, want %q", summary, want)
	}
	doc, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(doc, []byte("Third-party notices for gitlab-mcp-server\n")) || !bytes.HasSuffix(doc, []byte("Copyright Toml\n")) {
		t.Errorf("the written file is not the rendered notices:\n%s", doc)
	}

	short := base(filepath.Join(t.TempDir(), "x"))
	short.targets = []string{"linux/amd64"}
	noEnv := base(filepath.Join(t.TempDir(), "x"))
	noEnv.goroot = ""
	noEnv.goEnv = func() ([]byte, error) { return []byte("{}"), nil }
	badGOROOT := base(filepath.Join(t.TempDir(), "x"))
	badGOROOT.goroot = fixtureGOROOT(t, "go1.26.4")
	unwritable := base(filepath.Join(t.TempDir(), "missing", "THIRD_PARTY_NOTICES"))
	unreadable := base(filepath.Join(t.TempDir(), "x"))
	unreadable.readInfo = func(string) (*debug.BuildInfo, error) { return nil, os.ErrInvalid }
	for _, tc := range []struct {
		name string
		cfg  config
		want string
	}{
		{"a target short", short, "-targets names linux/amd64"},
		{"go env names no GOROOT", noEnv, "go env reports no GOROOT"},
		{"GOROOT of another toolchain", badGOROOT, "is go1.26.4"},
		{"an unwritable destination", unwritable, "missing"},
		{"a binary with no build information", unreadable, os.ErrInvalid.Error()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, runErr := run(tc.cfg); runErr == nil || !strings.Contains(runErr.Error(), tc.want) {
				t.Fatalf("run = %v, want an error containing %q", runErr, tc.want)
			}
		})
	}
}

// TestSplitList_DropsEmptyItems pins how -targets is read.
func TestSplitList_DropsEmptyItems(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"":                                "[]",
		"linux/amd64":                     "[linux/amd64]",
		" linux/amd64 ,, windows/arm64 ,": "[linux/amd64 windows/arm64]",
	} {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			got := splitList(in)
			if s := "[" + strings.Join(got, " ") + "]"; s != want {
				t.Errorf("splitList(%q) = %s, want %s", in, s, want)
			}
		})
	}
}

// TestMain_ExitsWithTheCodeRunMainDecides covers the entry point: main hands
// the process arguments to runMain and exits with what it returns.
func TestMain_ExitsWithTheCodeRunMainDecides(t *testing.T) {
	saved, savedArgs := exitProcess, os.Args
	t.Cleanup(func() { exitProcess, os.Args = saved, savedArgs })

	code := -1
	exitProcess = func(c int) { code = c }
	os.Args = []string{toolName, "-nope"}
	main()
	if code != 2 {
		t.Fatalf("main exited %d, want 2", code)
	}
}
