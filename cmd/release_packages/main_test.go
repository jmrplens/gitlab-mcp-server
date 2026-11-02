// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// main_test.go covers the command line: which subcommand a run makes, the
// usage errors refused before any starts, the manifest subcommand's own
// failures, and the exit code main hands the process.
package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunIn_RunsTheSubcommandItIsNamed covers the first argument and the
// number of arguments each subcommand takes: each is reached by its name,
// help is not an error, and anything else is refused with exit 2 and the
// usage.
func TestRunIn_RunsTheSubcommandItIsNamed(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		args   []string
		want   int
		stdout string
		stderr string
	}{
		{name: "no subcommand", args: nil, want: 2, stderr: "usage: release_packages mcpb VERSION"},
		{name: "an unknown subcommand", args: []string{"pack"}, want: 2, stderr: `release_packages: unknown subcommand "pack"`},
		{name: "-h", args: []string{"-h"}, want: 0, stdout: "usage: release_packages mcpb VERSION"},
		{name: "-help", args: []string{"-help"}, want: 0, stdout: "release_packages server-json-packages"},
		{name: "--help", args: []string{"--help"}, want: 0, stdout: "release_packages mcpb-manifest PLATFORM"},
		{name: "help", args: []string{"help"}, want: 0, stdout: "release_packages mcpb-manifest PLATFORM"},
		{name: "mcpb without a version", args: []string{"mcpb"}, want: 2, stderr: "release_packages: mcpb takes other arguments"},
		{name: "mcpb with three arguments", args: []string{"mcpb", "1", "dist", "x"}, want: 2, stderr: "release_packages: mcpb takes other arguments"},
		{name: "mcpb over an empty tree", args: []string{"mcpb", "1.0.0"}, want: 1, stderr: "ERROR: mcpb/manifest.json not found (run from the repository root)"},
		{name: "mcpb into another dist", args: []string{"mcpb", "1.0.0", "out"}, want: 1, stderr: "ERROR: mcpb/manifest.json not found"},
		{name: "mcpb-manifest without a platform", args: []string{"mcpb-manifest"}, want: 2, stderr: "mcpb-manifest takes other arguments"},
		{name: "mcpb-manifest with three arguments", args: []string{"mcpb-manifest", "linux", "a", "b"}, want: 2, stderr: "mcpb-manifest takes other arguments"},
		{name: "mcpb-manifest over an empty tree", args: []string{"mcpb-manifest", "linux"}, want: 1, stderr: "release_packages: mcpb/manifest.json could not be read as JSON"},
		{name: "server-json-packages with two arguments", args: []string{"server-json-packages", "a", "b"}, want: 2, stderr: "server-json-packages takes other arguments"},
		{name: "server-json-packages over an empty tree", args: []string{"server-json-packages"}, want: 1, stderr: "ERROR: server.json not found"},
		{name: "server-json-packages of another file", args: []string{"server-json-packages", "other.json"}, want: 1, stderr: "ERROR: other.json not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			code := runIn(t.TempDir(), tc.args, &stdout, &stderr, noEnv)
			if code != tc.want {
				t.Errorf("runIn(%q) = %d, want %d (stderr %q)", tc.args, code, tc.want, stderr.String())
			}
			if !strings.Contains(stdout.String(), tc.stdout) || !strings.Contains(stderr.String(), tc.stderr) {
				t.Errorf("runIn(%q) wrote %q on stdout and %q on stderr, want %q and %q", tc.args, stdout.String(), stderr.String(), tc.stdout, tc.stderr)
			}
			if tc.stdout == "" && stdout.Len() != 0 {
				t.Errorf("runIn(%q) wrote %q on stdout, want nothing", tc.args, stdout.String())
			}
		})
	}
}

// brokenWriter fails every write.
type brokenWriter struct{}

// Write fails.
func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("stdout is closed") }

// TestPrintPlatformManifest_ReadsAPathAndReportsAFailedWrite covers the
// manifest subcommand on an absolute path, on a manifest that is not JSON,
// and on a stdout it cannot write to.
func TestPrintPlatformManifest_ReadsAPathAndReportsAFailedWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	good := filepath.Join(dir, "manifest.json")
	writeFile(t, dir, "manifest.json", []byte(`{"server":{"mcp_config":{"command":"${__dirname}/s"}}}`))
	writeFile(t, dir, "bad.json", []byte(`{`))
	for _, tc := range []struct {
		name   string
		path   string
		stdout io.Writer
		want   int
		stderr string
	}{
		{name: "an absolute path", path: good, stdout: &bytes.Buffer{}, want: 0},
		{name: "a manifest that is not JSON", path: filepath.Join(dir, "bad.json"), stdout: &bytes.Buffer{}, want: 1, stderr: "bad.json could not be read as JSON: "},
		{name: "a stdout that refuses", path: good, stdout: brokenWriter{}, want: 1, stderr: "release_packages: stdout is closed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stderr bytes.Buffer
			if code := printPlatformManifest("/nowhere", "darwin", tc.path, tc.stdout, &stderr); code != tc.want || !strings.Contains(stderr.String(), tc.stderr) {
				t.Errorf("exit %d and stderr %q, want %d and %q", code, stderr.String(), tc.want, tc.stderr)
			}
		})
	}
}

// TestMain_HandsTheExitCodeToTheProcess covers main and runMain: the code
// the subcommand decides, from the working directory, is the one the process
// exits with. It replaces os.Args and the exit seam, so it does not run in
// parallel.
func TestMain_HandsTheExitCodeToTheProcess(t *testing.T) {
	previousArgs, previousExit := os.Args, exitProcess
	t.Cleanup(func() { os.Args, exitProcess = previousArgs, previousExit })

	code := -1
	exitProcess = func(c int) { code = c }
	os.Args = []string{toolName, "mcpb-manifest", "freebsd", filepath.Join("..", "..", "mcpb", "manifest.json")}
	main()
	if code != 1 {
		t.Errorf("main exited %d for a platform Claude Desktop does not report, want 1", code)
	}
	os.Args = []string{toolName}
	main()
	if code != 2 {
		t.Errorf("main exited %d with no subcommand, want 2", code)
	}
}
