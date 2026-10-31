// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// main_test.go drives the command end to end: go.mod and the document read,
// the document validated, the fixture module scanned against a database the
// test wrote, and the exit code the gate turns on.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/serialtypecheck" // serial type-checking under -race, golang/go#81122
)

// writeDocument writes doc into dir as the fixture's VEX document and returns
// its path.
func writeDocument(t *testing.T, dir string, doc document) string {
	t.Helper()

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("encoding the document: %v", err)
	}
	return writeFile(t, dir, filepath.FromSlash(defaultDocument), string(data))
}

// fixtureTree writes the fixture module with doc as its VEX document and
// returns the module directory.
func fixtureTree(t *testing.T, doc document) string {
	t.Helper()

	dir := writeFixtureModule(t)
	writeDocument(t, dir, doc)
	return dir
}

// TestRun_HoldsTheDocumentToTheDependencySet is the gate doing its job on
// the fixture: a document stating both advisories the scan reports without a
// call passes, and one stating only the first fails on the second.
func TestRun_HoldsTheDocumentToTheDependencySet(t *testing.T) {
	t.Parallel()

	both := fixtureDocument(moduleOnly.id, packageOf.id)
	both.Statements[1].Justification = "vulnerable_code_not_in_execute_path"
	db := writeVulnDB(t, moduleOnly, packageOf, calledFunc, neverRead)
	for _, tc := range []struct {
		name string
		doc  document
		code int
		want string
	}{
		{name: "both stated", doc: both, code: 0, want: "every statement holds"},
		{name: "one unaccounted", doc: fixtureDocument(moduleOnly.id), code: 1, want: "UNACCOUNTED " + packageOf.id + " stdlib@"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			code := runMain(context.Background(), []string{"-dir", fixtureTree(t, tc.doc), "-db", db, "./..."}, &stdout, &stderr)
			if code != tc.code || !strings.Contains(stdout.String(), tc.want) {
				t.Fatalf("exit %d, want %d and %q\nstdout:\n%s\nstderr:\n%s", code, tc.code, tc.want, stdout.String(), stderr.String())
			}
			if strings.Contains(stdout.String(), calledFunc.id) {
				t.Errorf("the report asks for a statement of an advisory whose function is called, which make govulncheck owns:\n%s", stdout.String())
			}
		})
	}
}

// TestRun_ADocumentThatDoesNotValidate_IsNotScanned covers the order of the
// two halves: the database here does not exist, so a scan would fail on
// stderr, and the report of the invalid document on stdout is what comes out.
func TestRun_ADocumentThatDoesNotValidate_IsNotScanned(t *testing.T) {
	t.Parallel()

	doc := fixtureDocument(moduleOnly.id)
	doc.Author = ""
	dir := fixtureTree(t, doc)
	var stdout, stderr bytes.Buffer
	absent := "file://" + filepath.ToSlash(filepath.Join(t.TempDir(), "absent"))
	code := run(context.Background(), auditConfig{dir: dir, document: filepath.Join(dir, filepath.FromSlash(defaultDocument)), db: absent, patterns: []string{"./..."}}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stdout.String(), "INVALID the document names no author") || stderr.Len() != 0 {
		t.Errorf("exit %d, want 1 with the problem on stdout and nothing on stderr\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
}

// TestRun_ARunThatCannotBeMade_FailsOnStderr covers each input failing: the
// go.mod, the document and the scan. A gate that could not check the
// document must not read as one that passed.
func TestRun_ARunThatCannotBeMade_FailsOnStderr(t *testing.T) {
	t.Parallel()

	dir := fixtureTree(t, fixtureDocument(moduleOnly.id))
	absent := "file://" + filepath.ToSlash(filepath.Join(t.TempDir(), "absent"))
	for _, tc := range []struct {
		name string
		cfg  auditConfig
		want string
	}{
		{name: "go.mod", cfg: auditConfig{dir: t.TempDir(), document: defaultDocument}, want: "go.mod"},
		{name: "document", cfg: auditConfig{dir: dir, document: "absent.json"}, want: "absent.json"},
		{name: "scan", cfg: auditConfig{dir: dir, document: defaultDocument, db: absent, patterns: []string{"./..."}}, want: "govulncheck over"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			if code := run(context.Background(), tc.cfg, &stdout, &stderr); code != 1 {
				t.Fatalf("exit %d, want 1", code)
			}
			if !strings.Contains(stderr.String(), tc.want) || stdout.Len() != 0 {
				t.Errorf("stderr %q lacks %q, or stdout carried a report: %q", stderr.String(), tc.want, stdout.String())
			}
		})
	}
}

// TestRunMain_ArgumentsItCannotRead covers the command line: -h is not an
// error, and an unknown flag is a usage error rather than a run.
func TestRunMain_ArgumentsItCannotRead(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		args []string
		want int
	}{
		{name: "help", args: []string{"-h"}, want: 0},
		{name: "unknown flag", args: []string{"-nope"}, want: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			if code := runMain(context.Background(), tc.args, &stdout, &stderr); code != tc.want {
				t.Errorf("runMain(%q) = %d, want %d", tc.args, code, tc.want)
			}
		})
	}
}

// TestMain_HandsTheExitCodeToTheProcess covers main itself through the exit
// seam, and the default patterns, from a directory with no go.mod so it
// stops before scanning.
func TestMain_HandsTheExitCodeToTheProcess(t *testing.T) {
	previousArgs, previousExit := os.Args, exitProcess
	t.Cleanup(func() { os.Args, exitProcess = previousArgs, previousExit })

	os.Args = []string{toolName, "-dir", t.TempDir()}
	code := -1
	exitProcess = func(c int) { code = c }
	main()
	if code != 1 {
		t.Fatalf("main exited %d, want 1 for a tree with no go.mod", code)
	}
}
