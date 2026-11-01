// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// main_test.go covers the command line: a tree it cannot read is refused,
// and the exit code main hands the process.
package main

import (
	"bytes"
	"maps"
	"strings"
	"testing"
)

// TestRun_AnEmptyTree_ReportsEveryManifestMissing verifies that a tree
// holding none of the manifests reports each of the three and exits 1.
func TestRun_AnEmptyTree_ReportsEveryManifestMissing(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := run(t.TempDir(), &stdout, &stderr)
	lines := strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n")
	if code != 1 || stdout.Len() != 0 || len(lines) != 3 {
		t.Fatalf("exit %d, stdout %q, stderr %q, want 1, nothing and three lines", code, stdout.String(), stderr.String())
	}
	for i, file := range []string{agentPluginPath, legacyPluginPath, mcpConfigPath} {
		t.Run(file, func(t *testing.T) {
			t.Parallel()

			if !strings.HasPrefix(lines[i], "ERROR: "+file+" could not be read as a JSON object: ") {
				t.Errorf("line %d is %q, want it to name %s", i, lines[i], file)
			}
		})
	}
}

// TestCheckManifests_AManifestThatIsNotAnObject_IsRefused covers JSON of
// another kind than the object every manifest is.
func TestCheckManifests_AManifestThatIsNotAnObject_IsRefused(t *testing.T) {
	t.Parallel()

	files := maps.Clone(readOpenpluginRecord(t).Cases[0].Files)
	files[mcpConfigPath] = "[]\n"
	got := checkManifests(layOut(t, files))
	if len(got) != 1 || !strings.HasPrefix(got[0], "mcp.json could not be read as a JSON object: ") {
		t.Errorf("problems %q, want mcp.json refused alone", got)
	}
}

// TestCheckManifests_ANameThatIsNotAString_IsLeftToTheSchema verifies that
// the name pattern held in code judges only a name that is a string: one of
// another kind is the schema's type problem and the name rule's, and not a
// pattern problem besides.
func TestCheckManifests_ANameThatIsNotAString_IsLeftToTheSchema(t *testing.T) {
	t.Parallel()

	files := maps.Clone(readOpenpluginRecord(t).Cases[0].Files)
	files[agentPluginPath] = strings.Replace(files[agentPluginPath], `"name": "gitlab-mcp-server"`, `"name": 7`, 1)
	got := checkManifests(layOut(t, files))
	if len(got) != 2 || !strings.HasPrefix(got[0], "plugin.json does not match the schema it declares") || got[1] != "plugin.json name must be gitlab-mcp-server" {
		t.Errorf("problems %q, want the schema's and the name rule's alone", got)
	}
}

// TestMain_HandsTheExitCodeToTheProcess covers main, which checks the
// working directory: an empty one fails. It changes the working directory
// and the exit seam, so it does not run in parallel.
func TestMain_HandsTheExitCodeToTheProcess(t *testing.T) {
	previousExit := exitProcess
	t.Cleanup(func() { exitProcess = previousExit })
	t.Chdir(t.TempDir())

	code := -1
	exitProcess = func(c int) { code = c }
	main()
	if code != 1 {
		t.Errorf("main exited %d in an empty directory, want 1", code)
	}
}
