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
		}, "ERROR: COVERAGE_MIN=most is not a finite number\n"},
		{"a minimum no total is below", func(_ *testing.T, b *bench) {
			b.session.environ = append(b.session.environ, "COVERAGE_MIN=NaN")
		}, "ERROR: COVERAGE_MIN=NaN is not a finite number\n"},
		{"an infinite minimum", func(_ *testing.T, b *bench) {
			b.session.environ = append(b.session.environ, "COVERAGE_MIN=-Inf")
		}, "ERROR: COVERAGE_MIN=-Inf is not a finite number\n"},
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

// TestReadSettings_ADotenvThatCannotBeParsed_ShowsNothingOfIt verifies that a
// .env the parser stops at is refused with a message that carries nothing the
// file holds, on either stream: the parser's own error quotes the rest of the
// file from where it stopped, or the value that does not end, and both hold
// the secrets.
func TestReadSettings_ADotenvThatCannotBeParsed_ShowsNothingOfIt(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, dotenv string }{
		{"a command before the secrets", "[ -f other ] && . ./other\nSONARQUBE_TOKEN=secret-one\nGITLAB_TOKEN=secret-two\n"},
		{"a command after the secrets", "SONARQUBE_TOKEN=secret-one\n. ./secret-two\n"},
		{"a value that does not end", "GITLAB_TOKEN=secret-one\nSONARQUBE_TOKEN=\"secret-two\n"},
		{"a name alone", "SONARQUBE_TOKEN=secret-one\nsecret-two"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			run := usual(t)
			run.Dotenv = &tc.dotenv
			b := newBench(t, run)
			const want = "ERROR: .env could not be parsed: a line is not NAME=value or a quoted value does not end (the file is not shown, since it holds secrets)\n"
			if status := b.session.main(t.Context(), nil); status != 1 || b.stderr.String() != want {
				t.Errorf("status = %d, stderr = %q; want 1 and %q", status, b.stderr.String(), want)
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
