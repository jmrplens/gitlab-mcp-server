package golist

import (
	"os"
	"path/filepath"
	"runtime"
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
	if want := filepath.Join(runtime.GOROOT(), "bin"); filepath.Dir(got) != want { //nolint:staticcheck // The audited helper resolves the same GOROOT.
		t.Fatalf("Executable() = %q, want it under %q", got, want)
	}
	info, err := os.Stat(got)
	if err != nil || info.IsDir() {
		t.Fatalf("Executable() = %q, want an existing file (stat error %v)", got, err)
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
