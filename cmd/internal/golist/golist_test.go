package golist

import (
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseRows_ToolchainOutput_ReadsEveryField verifies the ordinary case:
// the rows a `go list -f Format` run writes come back as one entry per
// package, with each field where Format put it, and the trailing newline the
// toolchain always emits produces no entry of its own.
func TestParseRows_ToolchainOutput_ReadsEveryField(t *testing.T) {
	t.Parallel()

	got, err := ParseRows([]byte("/src/a\texample.com/a\ta\n/src/b/cmd\texample.com/b/cmd\tmain\n"))
	if err != nil {
		t.Fatalf("ParseRows() error = %v, want nil", err)
	}
	want := []PackageInfo{
		{Dir: "/src/a", ImportPath: "example.com/a", Name: "a"},
		{Dir: "/src/b/cmd", ImportPath: "example.com/b/cmd", Name: "main"},
	}
	if len(got) != len(want) {
		t.Fatalf("ParseRows() returned %d packages, want %d", len(got), len(want))
	}
	for i, pkg := range got {
		t.Run(pkg.ImportPath, func(t *testing.T) {
			t.Parallel()

			if pkg != want[i] {
				t.Fatalf("ParseRows()[%d] = %+v, want %+v", i, pkg, want[i])
			}
		})
	}
}

// TestParseRows_MalformedRows_AreSkippedOrRefused verifies the rows a real
// toolchain never emits between package rows: a blank interior line is
// skipped, and a line with any number of tabs but two is refused rather than
// read as a package, which is what keeps a warning a caller merged into the
// same stream out of the listing. Empty output is a listing of no packages,
// not an error, and comes back non-nil so a caller can range over it.
//
// The padded case is what holds the TrimSpace around the split, which no
// mutation can model and which every other case here survives: a line of
// spaces is not the empty string the loop skips, so without the trim it
// reaches the field count and the whole listing is refused.
func TestParseRows_MalformedRows_AreSkippedOrRefused(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		output  string
		wantErr bool
		wantLen int
	}{
		{name: "blank interior line is skipped", output: "d\tp\tn\n\nd2\tp2\tn2\n", wantLen: 2},
		{name: "empty output lists no packages", output: "", wantLen: 0},
		{name: "whitespace only lists no packages", output: "\n\n", wantLen: 0},
		{name: "padding lines around the rows are trimmed", output: "  \nd\tp\tn\n  \n", wantLen: 1},
		{name: "row without a tab is refused", output: "no-tabs-here\n", wantErr: true},
		{name: "row with a single tab is refused", output: "dir\tonly-one-field\n", wantErr: true},
		{name: "row with a fourth field is refused", output: "d\tp\tn\textra\n", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseRows([]byte(tc.output))
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "unexpected go list row") {
					t.Fatalf("ParseRows(%q) error = %v, want an unexpected-row refusal", tc.output, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRows(%q) error = %v, want nil", tc.output, err)
			}
			if got == nil {
				t.Fatal("ParseRows() = nil, want an empty slice a caller can range over")
			}
			if len(got) != tc.wantLen {
				t.Fatalf("ParseRows(%q) returned %d packages, want %d", tc.output, len(got), tc.wantLen)
			}
		})
	}
}

// TestExecutable_PointsAtTheRunningToolchain verifies the resolved Go tool
// path is an existing file inside the active GOROOT, which is what keeps every
// caller off the PATH.
func TestExecutable_PointsAtTheRunningToolchain(t *testing.T) {
	t.Parallel()

	got := Executable()
	if !filepath.IsAbs(got) {
		t.Fatalf("Executable() = %q, want an absolute path", got)
	}
	if want := filepath.Join(build.Default.GOROOT, "bin"); filepath.Dir(got) != want {
		t.Fatalf("Executable() = %q, want it under %q", got, want)
	}
	info, err := os.Stat(got)
	if err != nil || info.IsDir() {
		t.Fatalf("Executable() = %q, want an existing file (stat error %v)", got, err)
	}
}

// TestFormat_ToolchainListing_PutsEachFieldWhereParseRowsReadsIt drives the
// real toolchain the way cmd/godoc_tool does, `go list -f Format` through
// [Executable], and reads the row back with [ParseRows]. It is the one
// assertion that holds the template's field order to the order the parser
// assigns them in, and the only test here that names [Format] at all: every
// other one hands ParseRows a literal row, so reordering Format left this
// package's own suite green while both callers broke.
//
// The three values are pairwise distinct by construction (only Dir is a
// directory holding golist.go, only ImportPath ends in this package's path,
// only Name is the package clause), so no permutation of either side passes.
// It also proves Executable names a binary that runs, which the stat beside
// it cannot.
func TestFormat_ToolchainListing_PutsEachFieldWhereParseRowsReadsIt(t *testing.T) {
	t.Parallel()

	// #nosec G204 -- Executable returns the fixed Go tool path from the active toolchain, which is the property under test.
	output, err := exec.CommandContext(t.Context(), Executable(), "list", "-f", Format, ".").Output()
	if err != nil {
		t.Fatalf("go list -f %q .: %v", Format, err)
	}

	rows, err := ParseRows(output)
	if err != nil {
		t.Fatalf("ParseRows(%q) error = %v, want nil", output, err)
	}
	if len(rows) != 1 {
		t.Fatalf("go list -f Format . returned %d rows, want this package alone", len(rows))
	}

	got := rows[0]
	if _, statErr := os.Stat(filepath.Join(got.Dir, "golist.go")); statErr != nil {
		t.Errorf("Dir = %q, want the directory holding golist.go (stat error %v)", got.Dir, statErr)
	}
	if want := "/cmd/internal/golist"; !strings.HasSuffix(got.ImportPath, want) {
		t.Errorf("ImportPath = %q, want it to end in %q", got.ImportPath, want)
	}
	if got.Name != "golist" {
		t.Errorf("Name = %q, want the package clause golist", got.Name)
	}
}

// TestExecutable_WindowsGOOS_AppendsExeSuffix verifies the Windows branch,
// which appends the .exe suffix. It drives the runtimeGOOS seam because the
// tests run on a non-Windows host that never takes the branch on its own. The
// test is serial so its global override never overlaps the parallel
// TestExecutable_PointsAtTheRunningToolchain.
func TestExecutable_WindowsGOOS_AppendsExeSuffix(t *testing.T) {
	original := runtimeGOOS
	runtimeGOOS = "windows"
	t.Cleanup(func() { runtimeGOOS = original })

	if got := filepath.Base(Executable()); got != "go.exe" {
		t.Fatalf("Executable() base = %q, want go.exe", got)
	}
}
