// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build linux

// validate_npm_linux_test.go drives the install tier of validate-npm, the
// handshake through the launcher and the stop check under npx, with a
// stand-in for npm install that lays out what npm would and a stand-in npx
// that starts the installed server as a child of a shell, as npm runs the
// launcher through sh -c. The server is this test binary acting as the
// stand-in MCP server, which ends when its parent changes, the launcher's
// own parent watch.
package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// copyExecutable copies this test binary to file, so a process running it
// is told apart by its executable.
func copyExecutable(t *testing.T, file string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if mkdirErr := os.MkdirAll(filepath.Dir(file), 0o750); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	if writeErr := os.WriteFile(file, data, 0o700); writeErr != nil { //#nosec G306,G703 -- a test's own copy of itself, in its temporary directory
		t.Fatal(writeErr)
	}
}

// TestValidateNpm_Install_HandshakesAndStopsUnderNpx runs the install tier
// end to end against stand-ins: the launcher answers the handshake, and in
// both transports a SIGTERM to npx ends the server.
func TestValidateNpm_Install_HandshakesAndStopsUnderNpx(t *testing.T) {
	t.Parallel()

	host, ok := hostPlatform()
	if !ok {
		t.Skip("no release platform for this host")
	}
	root := npmFixture(t)
	work := t.TempDir()
	var stdout, stderr bytes.Buffer
	v := newValidation("validate-npm", root, "9.8.7", testFloor, &stdout, &stderr)
	// The stand-in npx starts whatever server the stand-in npm install laid
	// out, whose path is known once the install ran, so it reads the path
	// from a file the install writes.
	pathFile := filepath.Join(work, "server-path")
	v.npx = filepath.Join(work, "npx")
	writeScript(t, v.npx, fakeServerEnv+"=ok \"$(cat '"+pathFile+"')\" \"$@\"\nexit $?\n")
	v.run = func(dir string, _ []string, name string, args ...string) ([]byte, error) {
		if name != "npm" || args[0] != "install" {
			return nil, errors.New("unexpected " + name)
		}
		modules := filepath.Join(dir, "node_modules")
		server := filepath.Join(modules, npmScope, "gitlab-mcp-server-"+host.npmKey, binaryName)
		copyExecutable(t, server)
		writeFile(t, filepath.Join(modules, npmScope, "gitlab-mcp-server"), "package.json", []byte("{}"))
		writeScript(t, filepath.Join(modules, ".bin", binaryName), fakeServerEnv+"=ok exec \""+server+"\" \"$@\"\n")
		writeFile(t, work, "server-path", []byte(server))
		return nil, nil
	}
	n := &npmValidation{validation: v, packages: filepath.Join(root, npmPackageDir), main: filepath.Join(root, npmMainDir), work: work, pack: fakePack}
	n.install()
	if len(v.problems) != 0 {
		t.Fatalf("problems %q, want none", v.problems)
	}
	wants := []string{"answered initialize with version 9.8.7", "stop (stdio): SIGTERM to npx ended the server", "stop (http): SIGTERM to npx ended the server"}
	if missing := slices.DeleteFunc(wants, func(want string) bool { return strings.Contains(stdout.String(), want) }); len(missing) > 0 {
		t.Errorf("stdout %q, want it to say %q", stdout.String(), missing)
	}
}

// TestValidateNpm_StopUnderNpx_AServerThatOutlivesNpxIsAProblem covers the
// defect the stop check exists for: a server nothing told to stop, which
// kept its port after npx was gone up to 3.1.0, fails it in both transports.
func TestValidateNpm_StopUnderNpx_AServerThatOutlivesNpxIsAProblem(t *testing.T) {
	t.Parallel()

	host, ok := hostPlatform()
	if !ok {
		t.Skip("no release platform for this host")
	}
	project, work := t.TempDir(), t.TempDir()
	server := filepath.Join(project, "node_modules", npmScope, "gitlab-mcp-server-"+host.npmKey, binaryName)
	copyExecutable(t, server)
	var stdout, stderr bytes.Buffer
	v := newValidation("validate-npm", t.TempDir(), "9.8.7", testFloor, &stdout, &stderr)
	v.npx = filepath.Join(work, "npx")
	writeScript(t, v.npx, fakeServerEnv+"=stubborn-ok \""+server+"\" \"$@\"\nexit $?\n")
	n := &npmValidation{validation: v, work: work}
	n.stopUnderNpx(project, host)
	wantProblem(t, v.problems, "stop (stdio): the server outlived a SIGTERM to npx")
	wantProblem(t, v.problems, "stop (http): the server outlived a SIGTERM to npx")
}

// TestValidateNpm_Install_ReportsWhatGoesWrong covers an install that
// fails, one that leaves a package for another platform, and a stop check
// whose server never starts.
func TestValidateNpm_Install_ReportsWhatGoesWrong(t *testing.T) {
	t.Parallel()

	if _, ok := hostPlatform(); !ok {
		t.Skip("no release platform for this host")
	}
	for _, tc := range []struct {
		name    string
		install func(t *testing.T, dir string) error
		want    string
	}{
		{name: "the install fails", install: func(*testing.T, string) error { return errors.New("E404") }, want: "npm install failed"},
		{name: "nothing installed", install: func(*testing.T, string) error { return nil }, want: "npm installed nothing"},
		{name: "another platform installed", install: func(t *testing.T, dir string) error {
			t.Helper()
			scope := filepath.Join(dir, "node_modules", npmScope)
			writeFile(t, filepath.Join(scope, "gitlab-mcp-server"), "package.json", []byte("{}"))
			writeFile(t, filepath.Join(scope, "gitlab-mcp-server-other"), "package.json", []byte("{}"))
			return nil
		}, want: "a package for another platform was installed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := npmFixture(t)
			var stdout, stderr bytes.Buffer
			v := newValidation("validate-npm", root, "9.8.7", testFloor, &stdout, &stderr)
			v.run = func(dir string, _ []string, _ string, _ ...string) ([]byte, error) { return nil, tc.install(t, dir) }
			v.npx = filepath.Join(t.TempDir(), "no-npx")
			n := &npmValidation{validation: v, packages: filepath.Join(root, npmPackageDir), main: filepath.Join(root, npmMainDir), work: t.TempDir(), pack: fakePack}
			n.install()
			wantProblem(t, v.problems, tc.want)
		})
	}
}

// TestProcStat_ReadsTheRunningProcess covers the /proc readers over this
// process: its state and parent, and none for a process that is not there.
func TestProcStat_ReadsTheRunningProcess(t *testing.T) {
	t.Parallel()

	state, ppid, ok := procStat(os.Getpid())
	if !ok || ppid != os.Getppid() || state == "" {
		t.Errorf("procStat(self) = %q, %d, %t, want a state and parent %d", state, ppid, ok, os.Getppid())
	}
	if !alive(os.Getpid()) {
		t.Error("alive(self) = false")
	}
	if _, _, found := procStat(1 << 30); found || alive(1<<30) {
		t.Error("a process that does not exist reads as there")
	}
	if pid := serverUnder(os.Getpid(), "/no/such/executable"); pid != 0 {
		t.Errorf("serverUnder found %d for an executable nothing runs", pid)
	}
}
