// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package golist

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// fileRow writes a row of [FilesFormat] from its fields.
func fileRow(fields ...string) string {
	return strings.Join(fields, "\t") + "\n"
}

// TestParseFileRows_EveryField_IsReadWhereTheFormatPutsIt verifies the rows a
// listing writes: each field in its place, a list split into its names, the
// directories unquoted, whatever they hold, and written with slashes whatever
// the platform that listed them, a carriage return a Windows console added
// dropped, and the error unquoted, its lines kept.
func TestParseFileRows_EveryField_IsReadWhereTheFormatPutsIt(t *testing.T) {
	t.Parallel()
	output := fileRow(`"C:\\src\\m\\pkg"`, `"C:\\src\\m"`, "pkg", "example.com/m/pkg", "a.go b.go", "c.go", "a_test.go", "x_test.go", "fmt io", "testing", "example.com/m/pkg", "") +
		strings.TrimSuffix(fileRow(`"/a\tb /m/gone"`, "", "", "example.com/m/gone", "", "", "", "", "", "", "", `"no such package:\n\tgone"`), "\n") + "\r\n\n"
	got, err := ParseFileRows([]byte(output))
	if err != nil {
		t.Fatalf("ParseFileRows() error = %v", err)
	}
	want := []PackageFiles{
		{
			Dir: "C:/src/m/pkg", ModuleDir: "C:/src/m", Name: "pkg", ImportPath: "example.com/m/pkg",
			GoFiles: []string{"a.go", "b.go"}, CgoFiles: []string{"c.go"}, TestGoFiles: []string{"a_test.go"}, XTestGoFiles: []string{"x_test.go"},
			Imports: []string{"fmt", "io"}, TestImports: []string{"testing"}, XTestImports: []string{"example.com/m/pkg"},
		},
		{
			Dir: "/a\tb /m/gone", ImportPath: "example.com/m/gone", GoFiles: []string{}, CgoFiles: []string{}, TestGoFiles: []string{}, XTestGoFiles: []string{},
			Imports: []string{}, TestImports: []string{}, XTestImports: []string{}, Error: "no such package:\n\tgone",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseFileRows() =\n%+v\nwant\n%+v", got, want)
	}
}

// TestParseFileRows_NoRowsOrMalformedOnes_AreEmptyOrRefused verifies that no
// output is a listing of no packages a caller can range over, and that a row
// of any other number of fields, or a directory or an error that is not quoted,
// is refused.
func TestParseFileRows_NoRowsOrMalformedOnes_AreEmptyOrRefused(t *testing.T) {
	t.Parallel()
	if got, err := ParseFileRows(nil); err != nil || got == nil || len(got) != 0 {
		t.Errorf("ParseFileRows(nil) = %v, %v; want an empty listing", got, err)
	}
	for name, row := range map[string]string{
		"a field short":                fileRow(`"d"`, `"m"`, "n", "p", "", "", "", "", "", "", ""),
		"a field over":                 fileRow(`"d"`, `"m"`, "n", "p", "", "", "", "", "", "", "", "", ""),
		"an unquoted directory":        fileRow("d", `"m"`, "n", "p", "", "", "", "", "", "", "", ""),
		"an unquoted module directory": fileRow(`"d"`, "m", "n", "p", "", "", "", "", "", "", "", ""),
		"an unquoted error":            fileRow(`"d"`, `"m"`, "n", "p", "", "", "", "", "", "", "", "broken"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got, err := ParseFileRows([]byte(row)); err == nil || !strings.HasPrefix(err.Error(), "unexpected go list row: ") || got != nil {
				t.Errorf("ParseFileRows(%q) = %v, %v; want an unexpected-row refusal", row, got, err)
			}
		})
	}
}

// TestFilesFormat_ToolchainListing_IsWhatParseFileRowsReads drives the real
// toolchain over this package and over one that is not there, and holds the
// template's order to the parser's: the files of each kind, the package's own
// test files apart from any external ones, and an error a package cannot be
// loaded for.
func TestFilesFormat_ToolchainListing_IsWhatParseFileRowsReads(t *testing.T) {
	t.Parallel()
	// #nosec G204 -- Executable returns the fixed Go tool path from the active toolchain.
	output, err := exec.CommandContext(t.Context(), Executable(), "list", "-e", "-f", FilesFormat, ".", "./absent").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	rows, err := ParseFileRows(output)
	if err != nil || len(rows) != 2 {
		t.Fatalf("ParseFileRows(%q) = %d row(s), %v; want two", output, len(rows), err)
	}
	here, absent := rows[0], rows[1]
	if _, statErr := os.Stat(filepath.Join(here.Dir, "files.go")); statErr != nil {
		t.Errorf("Dir = %q, want the directory holding files.go (stat error %v)", here.Dir, statErr)
	}
	if _, statErr := os.Stat(filepath.Join(here.ModuleDir, "go.mod")); statErr != nil || here.Dir != here.ModuleDir+"/cmd/internal/golist" {
		t.Errorf("ModuleDir = %q and Dir = %q, want the module's root above this package (stat error %v)", here.ModuleDir, here.Dir, statErr)
	}
	if here.Name != "golist" || !strings.HasSuffix(here.ImportPath, "/cmd/internal/golist") || here.Error != "" {
		t.Errorf("row = %+v, want this package by name and path and no error", here)
	}
	// Held by a file of each kind rather than by the whole lists, since gobco
	// runs this test in a copy of the package beside files of its own.
	if !slices.Contains(here.GoFiles, "golist.go") || !slices.Contains(here.TestGoFiles, "golist_test.go") || len(here.CgoFiles)+len(here.XTestGoFiles) != 0 {
		t.Errorf("files = %+v, want golist.go among the sources, golist_test.go among the tests, and no other kind", here)
	}
	if len(here.Imports) == 0 || len(here.TestImports) == 0 || len(here.XTestImports) != 0 {
		t.Errorf("imports = %q, test imports = %q, external test imports = %q", here.Imports, here.TestImports, here.XTestImports)
	}
	if absent.Error == "" {
		t.Errorf("row of an absent package = %+v, want an error", absent)
	}
}

// TestFilesFormat_ACheckoutPathWithATab_IsReadBack verifies, where a file name
// may hold one, that a module below a directory whose name holds a tab is
// listed and read back as any other.
func TestFilesFormat_ACheckoutPathWithATab_IsReadBack(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("a file name holds no tab there")
	}
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "a\tb ")
	err = errors.Join(
		os.Mkdir(root, 0o750),
		os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/m\n\ngo 1.27\n"), 0o600),
		os.WriteFile(filepath.Join(root, "m.go"), []byte("package m\n"), 0o600),
	)
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G204 -- Executable returns the fixed Go tool path from the active toolchain.
	list := exec.CommandContext(t.Context(), Executable(), "list", "-e", "-f", FilesFormat, ".")
	list.Dir, list.Env = root, append(os.Environ(), "PWD="+root, "GOWORK=off", "GOFLAGS=")
	output, err := list.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	rows, err := ParseFileRows(output)
	if err != nil || len(rows) != 1 || rows[0].Dir != root || rows[0].ModuleDir != root {
		t.Errorf("ParseFileRows(%q) = %+v, %v; want one row of %q", output, rows, err, root)
	}
}

// TestPackageFiles_Lists_AreTheKindsInOrder verifies what the two lists the
// callers read are made of, and that neither shares the listing's own array,
// so appending to one leaves the listing as it was.
func TestPackageFiles_Lists_AreTheKindsInOrder(t *testing.T) {
	t.Parallel()
	pkg := PackageFiles{GoFiles: append(make([]string, 0, 8), "a.go"), CgoFiles: []string{"c.go"}, TestGoFiles: []string{"a_test.go"}, XTestGoFiles: []string{"x_test.go"}}
	if got, want := pkg.Built(), []string{"a.go", "c.go", "a_test.go", "x_test.go"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Built() = %q, want %q", got, want)
	}
	if got, want := pkg.Sources(), []string{"a.go", "c.go"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Sources() = %q, want %q", got, want)
	}
	if spare := pkg.GoFiles[:2]; spare[1] != "" {
		t.Errorf("a list was built over the listing's own array: %q", spare)
	}
}
