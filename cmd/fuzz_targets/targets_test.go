// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// listCall is one call a fake lister received.
type listCall struct {
	dir  string
	args []string
}

// fakeLister answers every call with listing, or with err when it is set,
// and records each call it receives.
func fakeLister(listing string, err error, calls *[]listCall) lister {
	return func(_ context.Context, dir string, args ...string) ([]byte, error) {
		*calls = append(*calls, listCall{dir: dir, args: args})
		if err != nil {
			return nil, err
		}
		return []byte(listing), nil
	}
}

// row writes one listing row the way listFormat does.
func row(path, module, dir, testFiles, xtestFiles string) string {
	return strings.Join([]string{path, module, dir, testFiles, xtestFiles}, "\t") + "\n"
}

// writeFiles writes each named file below dir with its content.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// TestIsFuzzName_Names_FollowTheGoCommandsRule verifies the rule the go
// command reads a fuzz target's name by: Fuzz alone, or Fuzz followed by any
// character that is not a lower-case letter, in or out of ASCII.
func TestIsFuzzName_Names_FollowTheGoCommandsRule(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		want bool
	}{
		{name: "Fuzz", want: true},
		{name: "FuzzMeterFor", want: true},
		{name: "Fuzz_lower", want: true},
		{name: "Fuzz1", want: true},
		{name: "FuzzÉcole", want: true},
		{name: "Fuzzy", want: false},
		{name: "Fuzzé", want: false},
		{name: "fuzzMeter", want: false},
		{name: "TestFuzzMeter", want: false},
		{name: "Fuz", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isFuzzName(tc.name); got != tc.want {
				t.Errorf("isFuzzName(%q) = %t, want %t", tc.name, got, tc.want)
			}
		})
	}
}

// TestFindTargets_Listing_ReadsEveryTestFileTheGoCommandBuilds verifies the
// discovery over a listing of three packages: the fuzz targets of a package's
// internal and external test files are found, sorted by name within it, and
// only top-level functions named as targets count, so a method, a Fuzzy, a
// test and a variable named like a target do not. A file in the directory the
// listing does not name is not read, which is how a file behind a build tag
// stays out, and a package without a target is passed over, the module root
// included. The listing is asked for every package of the module, in the
// directory given, with the row format the reader expects.
func TestFindTargets_Listing_ReadsEveryTestFileTheGoCommandBuilds(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"a/a_test.go": `package a

import "testing"

type suite struct{}

func (suite) FuzzMethod(f *testing.F) {}

func FuzzZeta(f *testing.F) {}

func Fuzzy(f *testing.F) {}

func TestFuzzLike(t *testing.T) {}

var FuzzVariable = 1

func FuzzAlpha(f *testing.F) {}
`,
		"a/a_ext_test.go": `package a_test

import "testing"

func FuzzMiddle(f *testing.F) {}
`,
		"a/tagged_test.go": `//go:build never

package a

import "testing"

func FuzzNever(f *testing.F) {}
`,
		"b/b_test.go": `package b

import "testing"

func TestB(t *testing.T) {}
`,
		"root_test.go": `package m

import "testing"

func TestRoot(t *testing.T) {}
`,
	})
	const module = "example.com/m"
	listing := row(module, module, root, "root_test.go", "") +
		"\r\n" +
		row(module+"/a", module, filepath.Join(root, "a"), "a_test.go", "a_ext_test.go") +
		row(module+"/b", module, filepath.Join(root, "b"), "b_test.go", "") +
		row(module+"/c", module, filepath.Join(root, "c"), "", "")
	var calls []listCall
	got, err := findTargets(context.Background(), "the-module", fakeLister(listing, nil, &calls))
	if err != nil {
		t.Fatalf("findTargets() error = %v", err)
	}
	want := []target{
		{pkg: "./a", name: "FuzzAlpha"},
		{pkg: "./a", name: "FuzzMiddle"},
		{pkg: "./a", name: "FuzzZeta"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findTargets() = %+v, want %+v", got, want)
	}
	wantCalls := []listCall{{dir: "the-module", args: []string{"list", "-f", listFormat, "./..."}}}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Errorf("lister calls = %+v, want %+v", calls, wantCalls)
	}
}

// TestFindTargets_Failures_SayWhatIsWrong verifies every way the discovery
// refuses rather than plans: a listing that failed, a row of another shape, a
// test file that does not parse or cannot be read, and a package holding a
// target that is not below its module's path, which no ./ name could reach.
func TestFindTargets_Failures_SayWhatIsWrong(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"broken_test.go": "package broken\n\nfunc FuzzBroken(f *testing.F) {\n",
		"fuzz_test.go":   "package m\n\nimport \"testing\"\n\nfunc FuzzRoot(f *testing.F) {}\n",
	})
	const module = "example.com/m"
	cases := []struct {
		name    string
		listing string
		err     error
		want    string
	}{
		{name: "the listing failed", err: errors.New("planted"), want: "go list: planted"},
		{name: "a row of four fields", listing: "a\tb\tc\td\n", want: `unexpected go list row "a\tb\tc\td"`},
		{name: "a row of six fields", listing: "a\tb\tc\td\te\tf\n", want: `unexpected go list row "a\tb\tc\td\te\tf"`},
		{
			name: "a file that does not parse", listing: row(module+"/broken", module, root, "broken_test.go", ""),
			want: "a test file the go command builds cannot be parsed: " + filepath.Join(root, "broken_test.go"),
		},
		{
			name: "a file that is not there", listing: row(module+"/gone", module, root, "", "gone_test.go"),
			want: "a test file the go command builds cannot be parsed: open " + filepath.Join(root, "gone_test.go"),
		},
		{
			name: "a target at the module root", listing: row(module, module, root, "fuzz_test.go", ""),
			want: `go list named "example.com/m", which is not below its module path "example.com/m"`,
		},
		{
			name: "a target in another module", listing: row("example.com/m2/x", module, root, "fuzz_test.go", ""),
			want: `go list named "example.com/m2/x", which is not below its module path "example.com/m"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var calls []listCall
			got, err := findTargets(context.Background(), ".", fakeLister(tc.listing, tc.err, &calls))
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) || got != nil {
				t.Errorf("findTargets() = %+v, %v; want nothing and an error starting %q", got, err, tc.want)
			}
		})
	}
}

// TestFindTargets_Repository_FindsItsTargetsWithTheRealToolchain runs the
// discovery over this repository with the go command itself, and checks it
// finds a target in each of the packages that hold one today, among them
// cmd/server, whose files differ between platforms, and internal/tenancy,
// whose targets span three files and keep a committed corpus.
func TestFindTargets_Repository_FindsItsTargetsWithTheRealToolchain(t *testing.T) {
	t.Parallel()
	got, err := findTargets(context.Background(), filepath.Join("..", ".."), goList)
	if err != nil {
		t.Fatalf("findTargets() error = %v", err)
	}
	for _, known := range []target{
		{pkg: "./cmd/server", name: "FuzzRefuseUnreadable"},
		{pkg: "./internal/finegrained", name: "FuzzDecodeGrant"},
		{pkg: "./internal/tenancy", name: "FuzzBudgetOn"},
		{pkg: "./internal/tenancy", name: "FuzzMeterFor"},
		{pkg: "./internal/tools/search", name: "FuzzFormatCodeMarkdown"},
		{pkg: "./internal/toolutil", name: "FuzzScanLogMarkup_RawOffset"},
	} {
		t.Run(known.pkg+" "+known.name, func(t *testing.T) {
			t.Parallel()
			found := false
			for _, each := range got {
				found = found || each == known
			}
			if !found {
				t.Errorf("findTargets() = %+v, which does not hold %+v", got, known)
			}
		})
	}
}

// TestGoList_MissingPackage_ReportsTheToolchainsWords verifies that a go
// command that failed is returned as an error carrying what it wrote to
// stderr, so the plan says why it could not list rather than only that.
func TestGoList_MissingPackage_ReportsTheToolchainsWords(t *testing.T) {
	t.Parallel()
	_, err := goList(context.Background(), ".", "list", "./nosuchpackagehere")
	if err == nil || !strings.Contains(err.Error(), "nosuchpackagehere") {
		t.Errorf("goList() error = %v, want one naming the missing package", err)
	}
}

// TestBuildMatrix_Targets_AreNamedOrRefusedOnACollision verifies that each
// target becomes one cell whose artifact is named after its package and its
// name, in the order given, and that two targets whose artifact names would
// be the same are refused rather than left to overwrite each other's upload.
func TestBuildMatrix_Targets_AreNamedOrRefusedOnACollision(t *testing.T) {
	t.Parallel()
	got, err := buildMatrix([]target{
		{pkg: "./internal/tenancy", name: "FuzzBusy"},
		{pkg: "./cmd/server", name: "FuzzRefuseUnreadable"},
	})
	if err != nil {
		t.Fatalf("buildMatrix() error = %v", err)
	}
	want := matrix{Include: []cell{
		{Package: "./internal/tenancy", Target: "FuzzBusy", Artifact: "fuzz-internal-tenancy-FuzzBusy"},
		{Package: "./cmd/server", Target: "FuzzRefuseUnreadable", Artifact: "fuzz-cmd-server-FuzzRefuseUnreadable"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildMatrix() = %+v, want %+v", got, want)
	}
	_, err = buildMatrix([]target{{pkg: "./a-b", name: "FuzzX"}, {pkg: "./a/b", name: "FuzzX"}})
	if wantErr := "FuzzX in ./a-b and FuzzX in ./a/b would both upload as fuzz-a-b-FuzzX"; err == nil || err.Error() != wantErr {
		t.Errorf("buildMatrix() error = %v, want %q", err, wantErr)
	}
}
