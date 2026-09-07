// time_helpers_test.go contains unit tests for time formatting and parsing helpers.
package toolutil

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestFormatTime_ValidRFC3339 verifies that FormatTime formats a valid
// RFC 3339 timestamp into a human-readable date string.
func TestFormatTime_ValidRFC3339(t *testing.T) {
	got := FormatTime("2026-03-20T15:45:00Z")
	want := "20 Mar 2026 15:45 UTC"
	if got != want {
		t.Errorf("FormatTime() = %q, want %q", got, want)
	}
}

// TestFormatTime_WithTimezone verifies that FormatTime correctly handles
// timestamps with explicit timezone offsets.
func TestFormatTime_WithTimezone(t *testing.T) {
	got := FormatTime("2026-03-20T10:45:00-05:00")
	want := "20 Mar 2026 15:45 UTC"
	if got != want {
		t.Errorf("FormatTime() = %q, want %q", got, want)
	}
}

// TestFormatTime_Empty verifies that FormatTime returns an empty string
// when given an empty input.
func TestFormatTime_Empty(t *testing.T) {
	got := FormatTime("")
	if got != "" {
		t.Errorf("FormatTime(\"\") = %q, want empty", got)
	}
}

// TestFormatTime_InvalidFormat verifies that FormatTime returns the string it
// was given when it cannot be parsed as a timestamp, unchanged for a value
// that renders as itself.
func TestFormatTime_InvalidFormat(t *testing.T) {
	input := "not-a-date"
	got := FormatTime(input)
	if got != input {
		t.Errorf("FormatTime(%q) = %q, want original input", input, got)
	}
}

// TestFormatTime_InvalidFormat_ContainsTheValue verifies that a value reaching
// the fallback cannot change the shape of the Markdown it is written into.
//
// Only a value that is not a timestamp gets this far, so the fallback is the
// one branch of this function that can be handed text somebody wrote. Every
// caller writes the result into a table cell or a list item, and the pipe, the
// newline and the '<' each end or reshape one.
func TestFormatTime_InvalidFormat_ContainsTheValue(t *testing.T) {
	got := FormatTime("a | b\nc <img src=x>")
	if strings.ContainsAny(got, "|\n<") {
		t.Errorf("FormatTime() = %q, which still carries a pipe, a newline or a '<'", got)
	}
	if !strings.Contains(got, "&#124;") || !strings.Contains(got, "&lt;") {
		t.Errorf("FormatTime() = %q, want the pipe and the angle bracket as entities", got)
	}
}

// TestParseOptionalTime_Valid verifies that ParseOptionalTime correctly
// parses a valid RFC 3339 timestamp string.
func TestParseOptionalTime_Valid(t *testing.T) {
	got := ParseOptionalTime("2026-01-01T00:00:00Z")
	if got == nil {
		t.Fatal("ParseOptionalTime() returned nil for valid input")
	}
}

// TestParseOptionalTime_Empty verifies that ParseOptionalTime returns a
// zero time when given an empty string.
func TestParseOptionalTime_Empty(t *testing.T) {
	got := ParseOptionalTime("")
	if got != nil {
		t.Errorf("ParseOptionalTime(\"\") = %v, want nil", got)
	}
}

// TestParseOptionalTime_Invalid verifies that ParseOptionalTime returns a
// zero time when given an unparseable timestamp string.
func TestParseOptionalTime_Invalid(t *testing.T) {
	got := ParseOptionalTime("invalid")
	if got != nil {
		t.Errorf("ParseOptionalTime(\"invalid\") = %v, want nil", got)
	}
}

// TestFormatTime_DateOnly verifies that FormatTime handles YYYY-MM-DD format.
func TestFormatTime_DateOnly(t *testing.T) {
	got := FormatTime("2026-03-20")
	want := "20 Mar 2026"
	if got != want {
		t.Errorf("FormatTime() = %q, want %q", got, want)
	}
}

// TestFormatTimePtr_Nil verifies that FormatTimePtr returns "" for a nil pointer.
func TestFormatTimePtr_Nil(t *testing.T) {
	got := FormatTimePtr(nil)
	if got != "" {
		t.Errorf("FormatTimePtr(nil) = %q, want \"\"", got)
	}
}

// TestFormatTimePtr_Valid verifies that FormatTimePtr formats a non-nil time as RFC 3339.
func TestFormatTimePtr_Valid(t *testing.T) {
	ts := time.Date(2026, 3, 20, 15, 45, 0, 0, time.UTC)
	got := FormatTimePtr(&ts)
	want := "2026-03-20T15:45:00Z"
	if got != want {
		t.Errorf("FormatTimePtr() = %q, want %q", got, want)
	}
}

// TestFormatISOTimePtr_Nil verifies that FormatISOTimePtr returns "" for a nil pointer.
func TestFormatISOTimePtr_Nil(t *testing.T) {
	got := FormatISOTimePtr(nil)
	if got != "" {
		t.Errorf("FormatISOTimePtr(nil) = %q, want \"\"", got)
	}
}

// TestFormatISOTimePtr_Valid verifies that FormatISOTimePtr formats a non-nil ISOTime as YYYY-MM-DD.
func TestFormatISOTimePtr_Valid(t *testing.T) {
	iso := gl.ISOTime(time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC))
	got := FormatISOTimePtr(&iso)
	want := "2026-03-20"
	if got != want {
		t.Errorf("FormatISOTimePtr() = %q, want %q", got, want)
	}
}

// formatTimePtrFuncDef matches a top-level Go function definition of
// formatTimePtr (with any return type). Used by the uniqueness guardrail.
var formatTimePtrFuncDef = regexp.MustCompile(`^func\s+formatTimePtr\b`)

// formatISOTimePtrFuncDef matches a top-level Go function definition of
// formatISOTimePtr (with any return type). Used by the uniqueness guardrail.
var formatISOTimePtrFuncDef = regexp.MustCompile(`^func\s+formatISOTimePtr\b`)

// timeHelperScan holds the result of the single shared repository sweep the
// two DEDUP-003 guardrail tests consume: offending "path:lineno" locations
// per helper regex. The sweep is repo-wide but runs once (sync.Once) and
// prunes non-source trees (.git, node_modules, dist, the Astro site), which
// previously dominated the walk with tens of thousands of irrelevant entries.
type timeHelperScan struct {
	formatTimePtr    []string
	formatISOTimePtr []string
	err              error
}

var (
	timeHelperScanOnce   sync.Once
	timeHelperScanResult timeHelperScan
)

// findDuplicateTimeHelper returns the duplicate-definition hits for re,
// excluding the canonical home (this package's source directory). Both
// guardrail regexes are matched in one shared walk.
func findDuplicateTimeHelper(t *testing.T, re *regexp.Regexp, skipDir string) []string {
	t.Helper()
	timeHelperScanOnce.Do(func() { timeHelperScanResult = scanRepoForTimeHelpers(t, skipDir) })
	if timeHelperScanResult.err != nil {
		t.Fatalf("walk repo: %v", timeHelperScanResult.err)
	}
	if re == formatISOTimePtrFuncDef {
		return timeHelperScanResult.formatISOTimePtr
	}
	return timeHelperScanResult.formatTimePtr
}

// scanRepoForTimeHelpers performs the single repository sweep, collecting the
// hits for both helper regexes at once.
func scanRepoForTimeHelpers(t *testing.T, skipDir string) timeHelperScan {
	t.Helper()
	repoRoot := findRepoRoot(t)
	prunedDirs := map[string]struct{}{
		".git": {}, "node_modules": {}, "dist": {}, "site": {},
	}
	var scan timeHelperScan
	scan.err = filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if _, pruned := prunedDirs[info.Name()]; pruned {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if filepath.Dir(path) == skipDir {
			return nil
		}
		f, err := os.Open(path) // #nosec G304,G122 -- test guardrail reads repo files via filepath.Walk
		if err != nil {
			return nil
		}
		defer func() { _ = f.Close() }()
		scanner := bufio.NewScanner(f)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if formatTimePtrFuncDef.MatchString(line) {
				rel, _ := filepath.Rel(repoRoot, path)
				scan.formatTimePtr = append(scan.formatTimePtr, rel+":"+itoa(lineNo))
			}
			if formatISOTimePtrFuncDef.MatchString(line) {
				rel, _ := filepath.Rel(repoRoot, path)
				scan.formatISOTimePtr = append(scan.formatISOTimePtr, rel+":"+itoa(lineNo))
			}
		}
		return nil
	})
	return scan
}

// findRepoRoot returns the absolute path to the repository root (the directory
// containing go.mod) by walking up from the test's working directory.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

// itoa is a small allocation-free integer-to-string helper used by the
// guardrail to format line numbers without pulling strconv into the test.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

// TestFormatTimePtr_UniqueAcrossRepo is the DEDUP-003 guardrail: the canonical
// formatTimePtr lives in this package as FormatTimePtr (exported). Any other
// package that defines an unexported `formatTimePtr` re-introduces duplication
// and confuses readers — call sites should use toolutil.FormatTimePtr or a
// distinctively-named helper (e.g. formatAuditDate).
func TestFormatTimePtr_UniqueAcrossRepo(t *testing.T) {
	canonicalDir, _ := filepath.Abs(".")
	hits := findDuplicateTimeHelper(t, formatTimePtrFuncDef, canonicalDir)
	if len(hits) > 0 {
		t.Fatalf("found %d duplicate formatTimePtr definition(s) outside %s:\n  %s\n"+
			"Use toolutil.FormatTimePtr or rename the helper to a purpose-specific name.",
			len(hits), canonicalDir, strings.Join(hits, "\n  "))
	}
}

// TestFormatISOTimePtr_UniqueAcrossRepo is the DEDUP-003 guardrail for the
// ISO-time variant — see TestFormatTimePtr_UniqueAcrossRepo.
func TestFormatISOTimePtr_UniqueAcrossRepo(t *testing.T) {
	canonicalDir, _ := filepath.Abs(".")
	hits := findDuplicateTimeHelper(t, formatISOTimePtrFuncDef, canonicalDir)
	if len(hits) > 0 {
		t.Fatalf("found %d duplicate formatISOTimePtr definition(s) outside %s:\n  %s\n"+
			"Use toolutil.FormatISOTimePtr or rename the helper to a purpose-specific name.",
			len(hits), canonicalDir, strings.Join(hits, "\n  "))
	}
}
