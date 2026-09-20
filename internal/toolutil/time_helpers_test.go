// time_helpers_test.go contains unit tests for time formatting and parsing helpers.
package toolutil

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/sourcewalk"
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

// TestRFC3339_WireFormInUTC verifies that RFC3339 writes the wire form GitLab
// itself uses: RFC 3339 in UTC, whatever zone the value carries, and nothing
// for a zero time, which is what a field GitLab did not send decodes to and
// would otherwise read as the year one.
func TestRFC3339_WireFormInUTC(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{name: "utc", in: time.Date(2026, 3, 20, 15, 45, 0, 0, time.UTC), want: "2026-03-20T15:45:00Z"},
		{name: "offset is converted", in: time.Date(2026, 3, 20, 10, 45, 0, 0, time.FixedZone("EST", -5*3600)), want: "2026-03-20T15:45:00Z"},
		{name: "zero time is absent", in: time.Time{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RFC3339(tt.in); got != tt.want {
				t.Errorf("RFC3339(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestRFC3339Ptr_NilIsAbsent verifies that the pointer form writes nothing
// for nil and the wire form otherwise, and that the deprecated FormatTimePtr
// is the same function under its old name, so a caller not yet migrated
// renders what it did.
func TestRFC3339Ptr_NilIsAbsent(t *testing.T) {
	ts := time.Date(2026, 3, 20, 15, 45, 0, 0, time.UTC)
	tests := []struct {
		name string
		in   *time.Time
		want string
	}{
		{name: "nil", in: nil, want: ""},
		{name: "value", in: &ts, want: "2026-03-20T15:45:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RFC3339Ptr(tt.in); got != tt.want {
				t.Errorf("RFC3339Ptr() = %q, want %q", got, tt.want)
			}
			if got := FormatTimePtr(tt.in); got != tt.want {
				t.Errorf("FormatTimePtr() = %q, want %q (the deprecated alias must agree)", got, tt.want)
			}
		})
	}
}

// TestFormatTimeValue_DisplayForm verifies that a time.Time renders in the
// same display form FormatTime gives the wire string, in UTC, and that a zero
// time is absent rather than the year one.
func TestFormatTimeValue_DisplayForm(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{name: "utc", in: time.Date(2026, 3, 20, 15, 45, 0, 0, time.UTC), want: "20 Mar 2026 15:45 UTC"},
		{name: "offset is converted", in: time.Date(2026, 3, 20, 10, 45, 0, 0, time.FixedZone("EST", -5*3600)), want: "20 Mar 2026 15:45 UTC"},
		{name: "zero time is absent", in: time.Time{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatTimeValue(tt.in); got != tt.want {
				t.Errorf("FormatTimeValue(%v) = %q, want %q", tt.in, got, tt.want)
			}
			if wire := RFC3339(tt.in); FormatTime(wire) != tt.want {
				t.Errorf("FormatTime(RFC3339(%v)) = %q, want %q: the two display paths disagree", tt.in, FormatTime(wire), tt.want)
			}
		})
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
// prunes what is not source of ours (node_modules, dist, the Astro site, and
// through sourcewalk every dot-directory and nested checkout), which
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
	return scanTreeForTimeHelpers(findRepoRoot(t), skipDir)
}

// scanTreeForTimeHelpers sweeps the tree at repoRoot for both helper regexes.
//
// It takes its root as an argument, and reports rather than aborts, so that
// [TestScanTreeForTimeHelpers_ANestedCheckout_IsNotCounted] can drive it over
// a tree it plants. That is the only reason it is separate from the caller
// above: what this sweep must not do cannot be shown against the repository
// itself, because the thing it must not read is whatever an agent happens to
// have checked out at the time.
func scanTreeForTimeHelpers(repoRoot, skipDir string) timeHelperScan {
	// These names are this sweep's own disinterest: generated output and the
	// built site hold no Go source of ours. What is not this repository's
	// source at all is sourcewalk's rule rather than this list's: a
	// dot-directory, and any nested checkout, which on a machine running the
	// parallel-agent tooling means the hundred and more worktrees under
	// .claude. That is why .git is not named here either.
	prunedDirs := map[string]struct{}{
		"node_modules": {}, "dist": {}, "site": {},
	}
	var scan timeHelperScan
	// The sweep is scoped to the repository root: every open goes through the
	// root, so a path that resolves outside it, whether a symlink planted in
	// the tree or one swapped in between the walk seeing an entry and opening
	// it, is refused rather than read.
	root, openErr := os.OpenRoot(repoRoot)
	if openErr != nil {
		scan.err = openErr
		return scan
	}
	defer func() { _ = root.Close() }()
	rootFS := root.FS()
	scan.err = fs.WalkDir(rootFS, ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// The rule is asked of rootFS rather than of the native path, so
			// the marker probe stays inside the root this sweep opened.
			if _, pruned := prunedDirs[d.Name()]; pruned || sourcewalk.SkipDirBelowRootFS(rootFS, rel) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		native := filepath.FromSlash(rel)
		if filepath.Join(repoRoot, filepath.Dir(native)) == skipDir {
			return nil
		}
		f, err := root.Open(native)
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
				scan.formatTimePtr = append(scan.formatTimePtr, native+":"+itoa(lineNo))
			}
			if formatISOTimePtrFuncDef.MatchString(line) {
				scan.formatISOTimePtr = append(scan.formatISOTimePtr, native+":"+itoa(lineNo))
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

// TestScanTreeForTimeHelpers_ANestedCheckout_IsNotCounted holds the two
// guardrails above to this repository's own source.
//
// The sweep they share walks from the repository root, and a checkout can
// contain other checkouts: the parallel-agent tooling puts a git worktree per
// agent under .claude/worktrees, and a developer can put one anywhere with
// `git worktree add`. Each is a complete copy of this repository on some other
// branch, and folding one into the sweep goes wrong twice. The visible way is
// cost, which is what was found: at 198 worktrees these guardrails swept 198
// copies and the package stopped finishing inside the ten-minute test timeout.
// The way worth guarding is the quiet one. A guardrail that fails names the
// file holding the duplicate, so a copy carrying another branch's code can
// fail it against a path that does not exist in this tree, and the reader is
// sent to open a file that is not there.
//
// So the assertion is that a definition inside a nested checkout is not
// counted, rather than that the walk survives one. The ordinary directory is
// the control and is what makes the rest mean anything: without a hit that
// must be found, every assertion here would also pass on a sweep that pruned
// the whole tree and read nothing.
func TestScanTreeForTimeHelpers_ANestedCheckout_IsNotCounted(t *testing.T) {
	base := t.TempDir()
	const definitions = "package helpers\n\nfunc formatTimePtr() string { return \"\" }\n\nfunc formatISOTimePtr() string { return \"\" }\n"

	// sequential: setup steps building one fixture tree, asserted by the cases below
	plant := func(dir string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(base, dir), 0o750); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(base, dir, "helpers.go"), []byte(definitions), 0o600); err != nil {
			t.Fatalf("WriteFile(%s/helpers.go) error = %v", dir, err)
		}
	}
	// The control: ordinary source of ours, which must be found.
	plant("ours")
	// A linked worktree in the shape the agent tooling leaves behind. Either
	// rule alone excludes it: it sits under a dot-directory, and it carries a
	// .git file of its own.
	worktree := filepath.Join(".claude", "worktrees", "agent")
	plant(worktree)
	if err := os.WriteFile(filepath.Join(base, worktree, ".git"), []byte("gitdir: /elsewhere\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(%s/.git) error = %v", worktree, err)
	}
	// A checkout whose name gives nothing away, which `git worktree add
	// ./scratch` produces. Only the marker rule excludes this one, so it is
	// what would still be read if the fix had been a name in a skip list.
	plant("scratch")
	if err := os.MkdirAll(filepath.Join(base, "scratch", ".git"), 0o750); err != nil {
		t.Fatalf("MkdirAll(scratch/.git) error = %v", err)
	}

	scan := scanTreeForTimeHelpers(base, "")
	if scan.err != nil {
		t.Fatalf("scanTreeForTimeHelpers() error = %v, want nil", scan.err)
	}

	want := filepath.Join("ours", "helpers.go")
	cases := []struct {
		name string
		hits []string
	}{
		{name: "formatTimePtr", hits: scan.formatTimePtr},
		{name: "formatISOTimePtr", hits: scan.formatISOTimePtr},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.hits) != 1 {
				t.Fatalf("%s: got %d hit(s) %v, want the one in %s alone: either a nested checkout was counted, or the walk read nothing at all",
					tc.name, len(tc.hits), tc.hits, want)
			}
			if !strings.HasPrefix(tc.hits[0], want+":") {
				t.Errorf("%s: got hit %q, want one in %s", tc.name, tc.hits[0], want)
			}
		})
	}
}
