// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package conditions

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// TestReadPlatform_ThreeLines_AreEnough verifies that go env's three answers
// are read whether or not a newline follows the last, and that a carriage
// return a Windows console added is no part of any.
func TestReadPlatform_ThreeLines_AreEnough(t *testing.T) {
	rec := readRecord(t)
	for _, answer := range []string{"linux\namd64\n/cache", "linux\r\namd64\r\n/cache\r\n"} {
		t.Run(answer, func(t *testing.T) {
			b := newBench(t, rec.Files)
			b.plant = answering("env", answer, 0, nil)
			status, stdout, stderr := b.measure(t.Context(), "./internal/plain")
			if status != Measured {
				t.Errorf("status = %d, stdout %q, stderr %q; want it measured", status, stdout, stderr)
			}
			if len(b.calls) != 1 || b.calls[0].GOMODCACHE != "/cache" {
				t.Errorf("ran %+v, want gobco once with the module cache /cache", b.calls)
			}
		})
	}
}

// row writes a listing of one package at dir, below module, with the files
// given, as go list -f golist.FilesFormat prints it.
func row(dir, module, files, tests, xtests string) string {
	return strings.Join([]string{strconv.Quote(dir), strconv.Quote(module), "plain", "example.com/m/internal/plain", files, "", tests, xtests, "", "", "", ""}, "\t") + "\n"
}

// TestReadReference_ALaterListingThatFails_IsRefused verifies that a beyond:
// gate whose reference platform cannot be listed at all is refused with the
// platform named, before gobco runs.
func TestReadReference_ALaterListingThatFails_IsRefused(t *testing.T) {
	rec := readRecord(t)
	b := newBench(t, rec.Files)
	b.environ = append(b.environ, "GOBCO_GATE=beyond:linux/amd64", "GOOS=windows")
	listings := 0
	b.plant = func(call recipe.Call) (bool, int, error) {
		if call.Args[0] == "list" {
			listings++
		}
		return listings == 2, 0, errors.New("planted")
	}
	status, _, stderr := b.measure(t.Context(), "./cmd/tool")
	if want := "gobco: go list could not list ./cmd/tool for linux/amd64: planted; refusing to measure\n"; status != NotMeasured || stderr != want {
		t.Errorf("status = %d, stderr = %q; want 1 and %q", status, stderr, want)
	}
	if len(b.calls) != 0 {
		t.Errorf("ran %+v, want nothing measured", b.calls)
	}
}

// TestReadStaging_FilesThatCannotBeRead_AreRefused verifies that a directory
// that cannot be listed, and a file of either kind the plan reads that is not
// there, stop the run rather than being read as a package with nothing to
// stage.
func TestReadStaging_FilesThatCannotBeRead_AreRefused(t *testing.T) {
	rec := readRecord(t)
	cases := []struct {
		name  string
		dir   string
		files string
		tests string
		xtest string
		want  string
	}{
		{name: "no directory", dir: "absent", want: "gobco: the files of ./internal/plain could not be listed: "},
		{name: "a source that went", dir: "internal/plain", files: "plain.go gone.go", tests: "plain_test.go", want: "gobco: a file of ./internal/plain could not be read: "},
		{name: "a test that went", dir: "internal/plain", files: "plain.go", tests: "gone_test.go", xtest: "plain_test.go", want: "gobco: a file of ./internal/plain could not be read: "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newBench(t, rec.Files)
			b.plant = answering("list", row(filepath.Join(b.root, tc.dir), b.root, tc.files, tc.tests, tc.xtest), 0, nil)
			status, _, stderr := b.measure(t.Context(), "./internal/plain")
			if status != NotMeasured || !strings.HasPrefix(stderr, tc.want) || !strings.HasSuffix(stderr, "; refusing to measure\n") {
				t.Errorf("status = %d, stderr = %q; want 1 and %q", status, stderr, tc.want)
			}
		})
	}
}

// TestReadStaging_WhatGobcoParses_IsEveryRegularGoFile verifies which entries
// of a directory read as left out: a file whose name ends in .go, or a link to
// one, that the go command does not build, and not a directory of such a name,
// a link to nothing or a file of another name.
func TestReadStaging_WhatGobcoParses_IsEveryRegularGoFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "package p\n")
	writeFile(t, dir, "draft.go", "package p\n")
	writeFile(t, dir, "notes.txt", "")
	writeFile(t, dir, "target", "package p\n")
	if err := os.Mkdir(filepath.Join(dir, "dir.go"), 0o750); err != nil {
		t.Fatal(err)
	}
	want := "draft.go"
	if runtime.GOOS != "windows" {
		linked := os.Symlink("target", filepath.Join(dir, "linked.go"))
		nowhere := os.Symlink("absent", filepath.Join(dir, "nowhere.go"))
		if err := errors.Join(linked, nowhere); err != nil {
			t.Fatal(err)
		}
		want = "draft.go linked.go"
	}
	m := &measurement{pkg: golist.PackageFiles{Dir: dir, GoFiles: []string{"a.go"}}, platform: platform{names: map[string]bool{}}}
	if err := m.readStaging(); err != nil {
		t.Fatal(err)
	}
	if got := joined(m.staged.leftOut); got != want {
		t.Errorf("left out %q, want %q", got, want)
	}
}

// TestReadStaging_AnExternalTestPackageOfACommand_IsGivenNoImport verifies
// that a package main, which nothing can import, is never staged for an
// external test package that does not import it, and that an importable one
// is.
func TestReadStaging_AnExternalTestPackageOfACommand_IsGivenNoImport(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "x_test.go", "package x_test\n")
	for name, want := range map[string]bool{"main": false, "x": true} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := &measurement{pkg: golist.PackageFiles{Dir: dir, Name: name, ImportPath: "example.com/m/x", XTestGoFiles: []string{"x_test.go"}}}
			if err := m.readStaging(); err != nil || m.staged.unimported != want || m.staged.needed() != want {
				t.Errorf("readStaging() of package %s: unimported = %t, %v; want %t", name, m.staged.unimported, err, want)
			}
		})
	}
}

// TestMisjudges_EachHeader_IsReadAsTheGoCommandReadsIt verifies which headers
// name more than the platform: a tag, a release or the compiler, in either
// spelling, alone or beside a platform, with a tab or a space after it, above
// the package clause only; and that an operating system, an architecture and
// unix are the platform, a name being read whole.
func TestMisjudges_EachHeader_IsReadAsTheGoCommandReadsIt(t *testing.T) {
	t.Parallel()
	p := platform{names: map[string]bool{"unix": true, "linux": true, "amd64": true}}
	cases := []struct {
		name, text string
		want       bool
	}{
		{"none", "package p\n", false},
		{"the platform", "//go:build linux && (amd64 || !unix)\n\npackage p\n", false},
		{"a tag", "//go:build e2e\n\npackage p\n", true},
		{"a tag beside the platform", "//go:build linux && e2e\n\npackage p\n", true},
		{"a release", "//go:build go1.22\n\npackage p\n", true},
		{"a name with an underscore", "//go:build linux_e2e\n\npackage p\n", true},
		{"an upper-case tag", "//go:build Z\n\npackage p\n", true},
		{"a tag of digits", "//go:build 9\n\npackage p\n", true},
		{"a lone letter", "//go:build a\n\npackage p\n", true},
		{"a lone z", "//go:build z\n\npackage p\n", true},
		{"a lone A", "//go:build A\n\npackage p\n", true},
		{"a lone 0", "//go:build 0\n\npackage p\n", true},
		{"a tab", "//go:build\te2e\n\npackage p\n", true},
		{"the old spelling", "// +build linux,e2e\n\npackage p\n", true},
		{"the old spelling, the platform", "// +build linux,amd64\n\npackage p\n", false},
		{"no blank after it", "//go:builde2e\n\npackage p\n", false},
		{"below the package clause", "package p\n\n//go:build e2e\n", false},
		{"below a clause with a tab", "package\tp\n//go:build e2e\n", false},
		{"below a word that only opens like a clause", "packages\n//go:build e2e\npackage p\n", true},
		{"no package clause", "//go:build linux\n", false},
		{"a platform tag before another", "//go:build linux\n//go:build e2e\npackage p\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := p.misjudges(tc.text); got != tc.want {
				t.Errorf("misjudges(%q) = %t, want %t", tc.text, got, tc.want)
			}
		})
	}
}
