// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// stamp_test.go covers the stamp a complete run leaves beside each run
// profile: where it is named, what it lists of the packages' sources, the
// trees it cannot stamp, and the record held to it, which takes a run
// profile only when its stamp is the tree's.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStampPath_NamesTheStampBesideTheProfile holds a run profile's stamp to
// its directory and stem: the .out the runner names its profiles with
// replaced, and the suffix added to a profile named otherwise.
func TestStampPath_NamesTheStampBesideTheProfile(t *testing.T) {
	tests := []struct{ profile, want string }{
		{profile: "dist/e2e-reports/e2e-ce-coverprofile.out", want: "dist/e2e-reports/e2e-ce-coverprofile.complete"},
		{profile: "dist/e2e-reports/e2e-ce-final-coverprofile.out", want: "dist/e2e-reports/e2e-ce-final-coverprofile.complete"},
		{profile: "run.prof", want: "run.prof.complete"},
	}
	for _, tc := range tests {
		t.Run(tc.profile, func(t *testing.T) {
			if got := stampPath(tc.profile); got != tc.want {
				t.Errorf("stampPath(%q) = %q, want %q", tc.profile, got, tc.want)
			}
		})
	}
}

// TestSourcesStamp_Packages_ListEverySourceButTheTests pins the stamp of a
// tree whose fixture package holds a second source, a test, a file that is
// not Go and a package of its own below it: every source of each package, the
// packages by directory and the files by name, and nothing else.
func TestSourcesStamp_Packages_ListEverySourceButTheTests(t *testing.T) {
	const second = "package fixture\n\nfunc B() {}\n"
	writeTree(t, map[string]string{
		"test/e2e/internal/fixture/b.go":          second,
		"test/e2e/internal/fixture/a_test.go":     "package fixture\n",
		"test/e2e/internal/fixture/notes.txt":     "not Go\n",
		"test/e2e/internal/fixture/inner/x.go":    "package inner\n",
		"test/e2e/internal/fixture/inner/x_go.md": "not Go either\n",
	})

	got, err := sourcesStamp(recordPackages)
	if err != nil {
		t.Fatalf("sourcesStamp() error = %v", err)
	}
	want := stampLine("test/e2e/internal/fixture/a.go", fixtureSource) +
		stampLine("test/e2e/internal/fixture/b.go", second) +
		stampLine("test/e2e/modeleval/c.go", modelevalSource)
	if string(got) != want {
		t.Errorf("stamp =\n%s\nwant\n%s", got, want)
	}
}

// TestSourcesStamp_ModuleRootPackage_IsStampedFromTheRoot holds the module's
// root package, which -packages spells ".", to the files at the module root:
// its sources and not its test or the packages below it, each named as
// sha256sum -c reads it from the root, and a run profile beside that stamp
// is the tree's for a record of the same package.
func TestSourcesStamp_ModuleRootPackage_IsStampedFromTheRoot(t *testing.T) {
	const rootSource = "package root\n"
	writeTree(t, map[string]string{"root.go": rootSource, "root_test.go": "package root\n"})
	packages := []string{importPath(".")}
	want := stampLine("root.go", rootSource)

	got, err := sourcesStamp(packages)
	if err != nil {
		t.Fatalf("sourcesStamp() error = %v", err)
	}
	if string(got) != want {
		t.Errorf("stamp =\n%s\nwant\n%s", got, want)
	}
	dir := t.TempDir()
	writeProfile(t, dir, "e2e-ce-coverprofile.complete", want)
	run := []source{{path: filepath.Join(dir, "e2e-ce-coverprofile.out"), half: onlineHalf}}
	if checkErr := checkStamps(run, packages); checkErr != nil {
		t.Errorf("checkStamps() with the root package's stamp = %v, want nil", checkErr)
	}
}

// TestSourcesStamp_PackagesInAnyOrder_StampTheSameSources holds the stamp to
// the packages it lists and not to the order -packages names them in, which
// go test -coverpkg does not depend on either: the packages named the other
// way round, or one of them twice, stamp the same sources the same way.
func TestSourcesStamp_PackagesInAnyOrder_StampTheSameSources(t *testing.T) {
	writeTree(t, nil)
	want := treeStamp(fixtureSource)
	tests := []struct {
		name     string
		packages []string
	}{
		{name: "in the order the Makefile names them", packages: recordPackages},
		{name: "the other way round", packages: []string{modelevalPkg, fixturePkg}},
		{name: "one of them twice", packages: []string{modelevalPkg, fixturePkg, modelevalPkg}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sourcesStamp(tc.packages)
			if err != nil {
				t.Fatalf("sourcesStamp() error = %v", err)
			}
			if string(got) != want {
				t.Errorf("stamp =\n%s\nwant\n%s", got, want)
			}
		})
	}
}

// TestCheckStamps_PassStampedWithThePackagesInAnotherOrder_IsTheTrees holds a
// record to the sources a pass stamped and not to the order the two named the
// packages in: a pass the runner stamped with E2E_COVERPKG naming them the
// other way round from the record's -packages is still the tree's.
func TestCheckStamps_PassStampedWithThePackagesInAnotherOrder_IsTheTrees(t *testing.T) {
	writeTree(t, nil)
	dir := t.TempDir()
	run := filepath.Join(dir, "e2e-ce-coverprofile.out")
	if err := writeStamp(stampPath(run), []string{modelevalPkg, fixturePkg}); err != nil {
		t.Fatalf("writeStamp() error = %v", err)
	}

	if err := checkStamps([]source{{path: run, half: onlineHalf}}, recordPackages); err != nil {
		t.Errorf("checkStamps() = %v, want nil", err)
	}
}

// TestSourcesStamp_TreeItCannotRead_IsRefused holds a tree the stamp cannot
// describe to an error naming the package: a package directory that is not
// there, and a source that cannot be read, here a directory named like one.
func TestSourcesStamp_TreeItCannotRead_IsRefused(t *testing.T) {
	tests := []struct {
		name     string
		packages []string
		dir      string
		want     string
	}{
		{name: "a package that is not there", packages: []string{fixturePkg, modelevalPkg + "/gone"}, want: "stamping the sources of test/e2e/modeleval/gone: "},
		{name: "a source it cannot read", packages: recordPackages, dir: "test/e2e/modeleval/dir.go", want: "stamping the sources of test/e2e/modeleval: "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, nil)
			if tc.dir != "" {
				if err := os.Mkdir(filepath.Join(root, filepath.FromSlash(tc.dir)), 0o750); err != nil {
					t.Fatal(err)
				}
			}

			_, err := sourcesStamp(tc.packages)
			checkErr := checkStamps(nil, tc.packages)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("sourcesStamp() error = %v, want it to hold %q", err, tc.want)
			}
			if checkErr == nil || !strings.Contains(checkErr.Error(), tc.want) {
				t.Errorf("checkStamps() error = %v, want the stamp's refusal %q", checkErr, tc.want)
			}
		})
	}
}

// TestCheckStamps_EveryRunProfile_IsHeldToTheTreesStamp holds every run
// profile of a record to its stamp, the first one in the order given and the
// later ones alike, and the offline profiles to none, since the offline half
// is run on the tree when the record is taken.
func TestCheckStamps_EveryRunProfile_IsHeldToTheTreesStamp(t *testing.T) {
	writeTree(t, nil)
	dir := t.TempDir()
	tree := treeStamp(fixtureSource)
	sources := []source{
		{path: filepath.Join(dir, "offline.out"), half: offlineHalf},
		{path: filepath.Join(dir, "e2e-ce-coverprofile.out"), half: onlineHalf},
		{path: filepath.Join(dir, "e2e-ce-final-coverprofile.out"), half: onlineHalf},
	}
	writeProfile(t, dir, "e2e-ce-coverprofile.complete", tree)

	if err := checkStamps(sources, recordPackages); err == nil || !strings.Contains(err.Error(), "e2e-ce-final-coverprofile.out has no stamp beside it") {
		t.Errorf("checkStamps() with the final pass unstamped = %v, want its profile refused", err)
	}
	writeProfile(t, dir, "e2e-ce-final-coverprofile.complete", tree)
	if err := checkStamps(sources, recordPackages); err != nil {
		t.Errorf("checkStamps() with both passes stamped = %v, want nil", err)
	}
}
