// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	yaml "go.yaml.in/yaml/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/fileheader"
)

// runCase is one repository and command line, and what the run must do with
// them. setup builds the repository and returns it and the arguments after
// -dir; verify, when set, inspects the tree the run left.
type runCase struct {
	name       string
	setup      func(t *testing.T) (dir string, args []string)
	wantCode   int
	wantStdout []string
	notStdout  []string
	wantStderr string
	verify     func(t *testing.T, dir string)
}

// check runs the case and reports every way the run departed from it: the
// exit code, each fragment stdout must hold and each it must not, and
// stderr, which must be empty or hold wantStderr.
func (c runCase) check(t *testing.T) {
	t.Helper()

	dir, args := c.setup(t)
	var stdout, stderr bytes.Buffer
	code := runMain(context.Background(), append([]string{"-dir", dir}, args...), &stdout, &stderr)
	if code != c.wantCode {
		t.Errorf("runMain() = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, c.wantCode, stdout.String(), stderr.String())
	}
	for _, want := range c.wantStdout {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout does not hold %q:\n%s", want, stdout.String())
		}
	}
	for _, unwanted := range c.notStdout {
		if strings.Contains(stdout.String(), unwanted) {
			t.Errorf("stdout holds %q:\n%s", unwanted, stdout.String())
		}
	}
	switch {
	case c.wantStderr == "" && stderr.Len() != 0:
		t.Errorf("stderr = %q, want it empty", stderr.String())
	case !strings.Contains(stderr.String(), c.wantStderr):
		t.Errorf("stderr = %q, want it to hold %q", stderr.String(), c.wantStderr)
	}
	if c.verify != nil {
		c.verify(t, dir)
	}
}

// readTree returns the content of the file at the slash-separated name
// under dir.
func readTree(t *testing.T, dir, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// The lines a run prints, as the cases below look for them.
const (
	headed       = slashC + slashL + "\npackage a\n"
	thirdParty   = "// Copyright 2009 The Go Authors. All rights reserved.\n\npackage x\n"
	cleanSummary = "file headers: %d source files, every one opening with the copyright and license header"
)

// summary spells the success line for n source files.
func summary(n int) string {
	return fmt.Sprintf(cleanSummary, n)
}

// TestRunMain_Repositories_JudgeEverySourceFile verifies a run end to end over
// real repositories: a clean tree passes, a planted file without the header
// fails with how to fix it, a file carrying another notice fails at its line,
// -fix writes the header and keeps each file's mode, a file with another
// notice is left as it was, and a file deleted from the working tree or a
// symbolic link is not judged.
func TestRunMain_Repositories_JudgeEverySourceFile(t *testing.T) {
	tests := []runCase{
		{
			name: "clean tree",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				return newRepository(t, map[string]string{
					"main.go":        headed,
					"scripts/run.sh": "#!/bin/sh\n" + hashC + hashL + "\necho a\n",
					"Makefile":       hashC + hashL + "\nall:\n",
					"README.md":      "# a\n",
				}), nil
			},
			wantStdout: []string{summary(3) + "\n"},
		},
		{
			name: "planted file without the header",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				return newRepository(t, map[string]string{
					"main.go":        headed,
					"cmd/new/new.go": "package main\n",
				}), nil
			},
			wantCode: 1,
			wantStdout: []string{
				"cmd/new/new.go: " + missingMessage + "\n",
				"file headers: 1 of 2 source files fail\n",
				"    " + fileheader.CopyrightStatement + "\n    " + fileheader.LicenseStatement + "\n",
				"make fix-file-headers",
			},
			verify: func(t *testing.T, dir string) {
				t.Helper()
				if got := readTree(t, dir, "cmd/new/new.go"); got != "package main\n" {
					t.Errorf("cmd/new/new.go = %q, want it left as it was without -fix", got)
				}
			},
		},
		{
			name: "file carrying another notice",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				return newRepository(t, map[string]string{"third/x.go": thirdParty}), nil
			},
			wantCode: 1,
			wantStdout: []string{
				"third/x.go: " + missingMessage + "\n",
				"third/x.go:1: " + noticeMessage + "\n",
				"file headers: 1 of 1 source files fail\n",
			},
		},
		{
			name: "header beside another notice",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				return newRepository(t, map[string]string{"a.go": headed + "\n// SPDX-License-Identifier: Apache-2.0\n"}), []string{"-fix"}
			},
			wantCode:   1,
			wantStdout: []string{"a.go:6: " + noticeMessage + "\n", "file headers: 1 of 1 source files fail\n"},
			notStdout:  []string{missingMessage, "header written"},
		},
		{
			name: "fix writes the header and keeps the mode",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				dir := newRepository(t, map[string]string{"a.go": "package a\n", "run.sh": "#!/bin/sh\necho a\n"})
				if err := os.Chmod(filepath.Join(dir, "run.sh"), 0o750); err != nil { //#nosec G302 -- an executable fixture script, so the run is seen keeping a mode the fixtures do not otherwise carry
					t.Fatal(err)
				}
				return dir, []string{"-fix"}
			},
			wantStdout: []string{"a.go: header written\n", "run.sh: header written\n", summary(2) + ", 2 of them written by this run\n"},
			verify: func(t *testing.T, dir string) {
				t.Helper()
				if got := readTree(t, dir, "a.go"); got != fileheader.Go+"package a\n" {
					t.Errorf("a.go = %q", got)
				}
				if got, want := readTree(t, dir, "run.sh"), "#!/bin/sh\n"+hashC+hashL+"\necho a\n"; got != want {
					t.Errorf("run.sh = %q, want %q", got, want)
				}
				info, err := os.Stat(filepath.Join(dir, "run.sh"))
				if err != nil {
					t.Fatal(err)
				}
				// Windows keeps no permission bits: os.Chmod only sets or clears
				// the read-only attribute, and a writable file stats as 0666.
				want := os.FileMode(0o750)
				if runtime.GOOS == "windows" {
					want = 0o666
				}
				if info.Mode().Perm() != want {
					t.Errorf("run.sh mode = %v, want %v kept", info.Mode().Perm(), want)
				}
			},
		},
		{
			name: "fix leaves a file carrying another notice",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				return newRepository(t, map[string]string{"a.go": "package a\n", "third/x.go": thirdParty}), []string{"-fix"}
			},
			wantCode:   1,
			wantStdout: []string{"a.go: header written\n", "third/x.go:1: " + noticeMessage + "\n", "file headers: 1 of 2 source files fail\n"},
			verify: func(t *testing.T, dir string) {
				t.Helper()
				if got := readTree(t, dir, "third/x.go"); got != thirdParty {
					t.Errorf("third/x.go = %q, want it left as it was", got)
				}
			},
		},
		{
			name: "fix on a clean tree writes nothing",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				return newRepository(t, map[string]string{"main.go": headed}), []string{"-fix"}
			},
			wantStdout: []string{summary(1) + "\n"},
			verify: func(t *testing.T, dir string) {
				t.Helper()
				if got := readTree(t, dir, "main.go"); got != headed {
					t.Errorf("main.go = %q, want it unchanged", got)
				}
			},
		},
		{
			name: "file deleted from the working tree and symbolic link",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				dir := newRepository(t, map[string]string{"main.go": headed, "gone.go": "package a\n", "target.txt": "package a\n"})
				gitFixture(t, dir, "add", "gone.go")
				if err := os.Remove(filepath.Join(dir, "gone.go")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("target.txt", filepath.Join(dir, "link.go")); err != nil {
					t.Fatal(err)
				}
				return dir, nil
			},
			wantStdout: []string{summary(1) + "\n"},
		},
		{
			name: "not a repository",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				isolateGit(t)
				dir := t.TempDir()
				t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
				return dir, nil
			},
			wantCode:   1,
			wantStderr: "audit_file_headers: could not list the files of ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, tt.check)
	}
}

// TestRunMain_FilesystemErrors_FailTheRunAndNameTheFile verifies a file that
// cannot be stated, read or rewritten fails the run with the reason on
// stderr, while the rest of the tree is still judged. The run's user can read
// anything the tests create when it is root, so the seams stand in for the
// filesystem.
func TestRunMain_FilesystemErrors_FailTheRunAndNameTheFile(t *testing.T) {
	failOn := func(name string) error {
		if filepath.Base(name) == "bad.go" {
			return errors.New("planted failure")
		}
		return nil
	}
	tests := []struct {
		name    string
		args    []string
		install func()
	}{
		{name: "lstat", install: func() {
			lstat = func(name string) (fs.FileInfo, error) {
				if err := failOn(name); err != nil {
					return nil, err
				}
				return os.Lstat(name)
			}
		}},
		{name: "read", install: func() {
			readFile = func(name string) ([]byte, error) {
				if err := failOn(name); err != nil {
					return nil, err
				}
				return os.ReadFile(name)
			}
		}},
		{name: "write", args: []string{"-fix"}, install: func() {
			writeFile = func(name string, data []byte, perm fs.FileMode) error {
				if err := failOn(name); err != nil {
					return err
				}
				return os.WriteFile(name, data, perm)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() { lstat, readFile, writeFile = os.Lstat, os.ReadFile, os.WriteFile })
			tt.install()
			runCase{
				setup: func(t *testing.T) (string, []string) {
					t.Helper()
					return newRepository(t, map[string]string{"bad.go": "package a\n", "main.go": headed}), tt.args
				},
				wantCode:   1,
				wantStdout: []string{"file headers: 1 of 2 source files fail\n"},
				wantStderr: "bad.go: planted failure",
			}.check(t)
		})
	}
}

// TestRunMain_Arguments_RefuseWhatTheyCannotUse verifies an unknown flag and
// a positional argument end the run with the usage status before anything is
// read.
func TestRunMain_Arguments_RefuseWhatTheyCannotUse(t *testing.T) {
	tests := []struct {
		name, wantStderr string
		args             []string
	}{
		{name: "unknown flag", args: []string{"-no-such-flag"}, wantStderr: "flag provided but not defined"},
		{name: "positional argument", args: []string{"main.go"}, wantStderr: "audit_file_headers: takes no arguments"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := runMain(context.Background(), tt.args, &stdout, &stderr); code != 2 {
				t.Errorf("runMain() = %d, want 2", code)
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to hold %q", stderr.String(), tt.wantStderr)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing", stdout.String())
			}
		})
	}
}

// TestMain_UnknownFlag_ExitsWithTheUsageStatus verifies main hands the status
// runMain returns to the process.
func TestMain_UnknownFlag_ExitsWithTheUsageStatus(t *testing.T) {
	savedArgs, savedExit := os.Args, osExit
	t.Cleanup(func() { os.Args, osExit = savedArgs, savedExit })
	os.Args = []string{toolName, "-no-such-flag"}
	got := -1
	osExit = func(code int) { got = code }
	main()
	if got != 2 {
		t.Errorf("main() exit = %d, want 2", got)
	}
}

// readRepositoryFile returns a file of this repository, which the contracts
// below hold to the command.
func readRepositoryFile(t *testing.T, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestMakefile_FileHeaderTargets_RunTheCommand holds the make targets to the
// command: check-file-headers runs the gate, fix-file-headers its fix mode,
// and make analyze runs the gate as its thirty-third step, with every step
// numbered out of the current total.
func TestMakefile_FileHeaderTargets_RunTheCommand(t *testing.T) {
	makefile := readRepositoryFile(t, "Makefile")
	tests := []struct{ name, want string }{
		{name: "check", want: "\ncheck-file-headers:\n\tgo run ./cmd/audit_file_headers/\n"},
		{name: "fix", want: "\nfix-file-headers:\n\tgo run ./cmd/audit_file_headers/ -fix\n"},
		{name: "analyze", want: "\trun_check \"[33/34] copyright and license headers\" go run ./cmd/audit_file_headers/; \\\n"},
		{name: "first step renumbered", want: "\trun_check \"[1/34] golangci-lint config verify\""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(makefile, tt.want) {
				t.Errorf("Makefile does not hold %q", tt.want)
			}
		})
	}
	if strings.Contains(makefile, "/31] ") || strings.Contains(makefile, "/32] ") || strings.Contains(makefile, "/33] ") {
		t.Error("Makefile still numbers an analyze step out of an earlier total")
	}
}

// TestCIWorkflow_GeneratedJob_ChecksTheFileHeaders holds ci.yml to running
// the gate on every push and pull request, in the job that already holds the
// tree's other source rules.
func TestCIWorkflow_GeneratedJob_ChecksTheFileHeaders(t *testing.T) {
	var workflow struct {
		Jobs map[string]struct {
			If    string `yaml:"if"`
			Steps []struct {
				If  string `yaml:"if"`
				Run string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(readRepositoryFile(t, ".github/workflows/ci.yml")), &workflow); err != nil {
		t.Fatal(err)
	}
	job, ok := workflow.Jobs["generated"]
	if !ok {
		t.Fatal("ci.yml has no generated job")
	}
	ran := false
	for _, step := range job.Steps {
		ran = ran || (step.Run == "make check-file-headers" && step.If == "")
	}
	if job.If != "" || !ran {
		t.Errorf("ci.yml's generated job (if %q) does not run make check-file-headers unconditionally", job.If)
	}
}
