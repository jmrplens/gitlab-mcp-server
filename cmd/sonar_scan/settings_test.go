// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// TestReadSettings_WhatCannotBeRead_IsRefusedBeforeAnythingRuns verifies that
// each setting and file the run needs is read before it starts, and says what
// is wrong with it.
func TestReadSettings_WhatCannotBeRead_IsRefusedBeforeAnythingRuns(t *testing.T) {
	t.Parallel()
	planted := errors.New("planted")
	asked := func(word string) func(recipe.Call) (bool, int, error) {
		return func(call recipe.Call) (bool, int, error) { return slices.Contains(call.Args, word), 0, planted }
	}
	cases := []struct {
		name    string
		prepare func(t *testing.T, b *bench)
		want    string
	}{
		{"no git", func(_ *testing.T, b *bench) {
			b.session.lookPath = func(string) (string, error) { return "", planted }
		}, "ERROR: find git: planted\n"},
		{"no repository", func(_ *testing.T, b *bench) { b.refuse = asked("--show-toplevel") }, "ERROR: planted\n"},
		{"no branch", func(_ *testing.T, b *bench) { b.refuse = asked("--abbrev-ref") }, "ERROR: planted\n"},
		{"a .env that is no file", func(t *testing.T, b *bench) {
			t.Helper()
			path := filepath.Join(b.root, ".env")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o750); err != nil {
				t.Fatal(err)
			}
		}, "ERROR: .env could not be read: "},
		{"no properties", func(t *testing.T, b *bench) {
			t.Helper()
			if err := os.Remove(filepath.Join(b.root, "sonar-project.properties")); err != nil {
				t.Fatal(err)
			}
		}, "ERROR: open "},
		{"no project", func(t *testing.T, b *bench) {
			t.Helper()
			b.write(t, "sonar-project.properties", "sonar.organization=x\n")
		}, "ERROR: sonar-project.properties: no sonar.projectKey= line\n"},
		{"a minimum that is no number", func(_ *testing.T, b *bench) {
			b.session.environ = append(b.session.environ, "COVERAGE_MIN=most")
		}, "ERROR: COVERAGE_MIN=most is not a number\n"},
		{"a timeout that is no number", func(_ *testing.T, b *bench) {
			b.session.environ = append(b.session.environ, "SONAR_POLL_TIMEOUT=4m")
		}, "ERROR: SONAR_POLL_TIMEOUT=4m is not a whole number of seconds\n"},
		{"an interval that is no number", func(_ *testing.T, b *bench) {
			b.session.environ = append(b.session.environ, "SONAR_POLL_INTERVAL=-1")
		}, "ERROR: SONAR_POLL_INTERVAL=-1 is not a whole number of seconds\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := newBench(t, usual(t))
			tc.prepare(t, b)
			if status := b.session.main(t.Context(), nil); status != 1 || !strings.HasPrefix(b.stderr.String(), tc.want) {
				t.Errorf("status = %d, stderr = %q; want 1 and %q", status, b.stderr.String(), tc.want)
			}
			if b.stdout.Len()+len(b.calls)+len(b.requests) != 0 {
				t.Errorf("stdout = %q, ran %+v, sent %+v; want nothing", b.stdout.String(), b.calls, b.requests)
			}
		})
	}
}

// TestReadSettings_AnotherDirectory_ReadsTheSameRepository verifies that the
// command finds the repository's root from a directory below it, and the
// defaults of what the environment does not set.
func TestReadSettings_AnotherDirectory_ReadsTheSameRepository(t *testing.T) {
	t.Parallel()
	b := newBench(t, usual(t))
	below := filepath.Join(b.root, "below")
	if err := os.Mkdir(below, 0o750); err != nil {
		t.Fatal(err)
	}
	b.session.dir = below
	b.session.environ = append(b.session.environ, "SONAR_HOST_URL=")
	b.session.lookPath = exec.LookPath
	read, err := b.session.readSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(read.root)
	if err != nil {
		t.Fatal(err)
	}
	if root != b.root || read.host != "https://sonarcloud.io" || read.coverageMin != "90" || read.branch != "trunk" || read.projectKey != "fixture_key" {
		t.Errorf("readSettings() = %+v, want the repository at %s and the defaults", read, b.root)
	}
}
