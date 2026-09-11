// diff_position_test.go contains unit tests for unified diff parsing and
// position validation helpers.
package toolutil

import (
	"testing"
)

// sampleDiff is a unified diff with additions, removals, and context lines.
// Old file lines 7–17, new file lines 7–27.
const sampleDiff = `@@ -7,11 +7,21 @@ int16_t ppc_comm_globals_get_sharing_q_meas( void )
 int16_t ppc_comm_globals_get_sharing_p_ref( void )
 {
+    if ( AEMO_COMMS_STATE_OPT_AEMO_COMMUNICATION_FAULT == ppc_comm_globals_get_aemo_fsm_state() )
+    {
+        return (int16_t) 0;
+    }
+
     return control_loops_get_sharing_power_p();
 }

 int16_t ppc_comm_globals_get_sharing_q_ref( void )
 {
+    if ( AEMO_COMMS_STATE_OPT_AEMO_COMMUNICATION_FAULT == ppc_comm_globals_get_aemo_fsm_state() )
+    {
+        return (int16_t) 0;
+    }
+
     return control_loops_get_sharing_power_q();
 }
`

// simpleDiff has added, removed, and context lines for easy counting.
const simpleDiff = `@@ -1,5 +1,6 @@
 line1
-line2_old
+line2_new
+line2b_added
 line3
 line4
 line5
`

// TestParseDiffLines_SimpleDiff verifies that ParseDiffLines correctly parses
// a simple unified diff with added and removed lines into DiffLine entries.
func TestParseDiffLines_SimpleDiff(t *testing.T) {
	lines := ParseDiffLines(simpleDiff)
	if len(lines) == 0 {
		t.Fatal("expected parsed lines, got none")
	}

	// Expected structure:
	// 1: context old=1, new=1
	// 2: removed old=2, new=0
	// 3: added   old=0, new=2
	// 4: added   old=0, new=3
	// 5: context old=3, new=4
	// 6: context old=4, new=5
	// 7: context old=5, new=6

	want := []DiffLine{
		{OldLine: 1, NewLine: 1, Type: LineContext},
		{OldLine: 2, NewLine: 0, Type: LineRemoved},
		{OldLine: 0, NewLine: 2, Type: LineAdded},
		{OldLine: 0, NewLine: 3, Type: LineAdded},
		{OldLine: 3, NewLine: 4, Type: LineContext},
		{OldLine: 4, NewLine: 5, Type: LineContext},
		{OldLine: 5, NewLine: 6, Type: LineContext},
	}

	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d", len(lines), len(want))
	}
	for i, got := range lines {
		if got != want[i] {
			t.Errorf("line[%d] = %+v, want %+v", i, got, want[i])
		}
	}
}

// TestParseDiffLines_EmptyDiff verifies that ParseDiffLines returns an empty
// slice when given an empty diff string.
func TestParseDiffLines_EmptyDiff(t *testing.T) {
	lines := ParseDiffLines("")
	if len(lines) != 0 {
		t.Errorf("expected 0 lines for empty diff, got %d", len(lines))
	}
}

// TestParseDiffLines_SampleDiff verifies that ParseDiffLines handles a
// multi-hunk diff with context, addition, and removal lines.
func TestParseDiffLines_SampleDiff(t *testing.T) {
	lines := ParseDiffLines(sampleDiff)
	if len(lines) == 0 {
		t.Fatal("expected parsed lines for sample diff")
	}

	// Verify we have added lines (from the + lines in the diff)
	var addedCount, contextCount int
	for _, l := range lines {
		switch l.Type {
		case LineAdded:
			addedCount++
		case LineContext:
			contextCount++
		}
	}
	if addedCount != 10 {
		t.Errorf("addedCount = %d, want 10", addedCount)
	}
	if contextCount == 0 {
		t.Error("expected some context lines")
	}
}

// TestParseDiffLines_NewFileHunk_KeepsTheAddedLines verifies that a hunk whose
// old side is empty is parsed rather than skipped. "@@ -0,0 +1,n @@" is what
// git writes for a file the merge request adds.
//
// The guard before the switch exists to drop the metadata that precedes the
// first hunk header, where neither counter has been set yet. A new file leaves
// the old counter at zero for the whole hunk, so a guard that asked about
// either counter instead of both would return no lines at all, and every
// inline comment on a newly added file would be refused as outside the diff.
func TestParseDiffLines_NewFileHunk_KeepsTheAddedLines(t *testing.T) {
	lines := ParseDiffLines("@@ -0,0 +1,2 @@\n+alpha\n+beta\n")

	want := []DiffLine{
		{NewLine: 1, Type: LineAdded},
		{NewLine: 2, Type: LineAdded},
	}
	if len(lines) != len(want) {
		t.Fatalf("ParseDiffLines() = %+v (%d lines), want %d", lines, len(lines), len(want))
	}
	for i, got := range lines {
		if got != want[i] {
			t.Errorf("line[%d] = %+v, want %+v", i, got, want[i])
		}
	}
}

// TestValidateDiffPosition_AddedLine verifies position validation for a line
// that was added in the diff (new_line set, old_line zero).
func TestValidateDiffPosition_AddedLine(t *testing.T) {
	lines := ParseDiffLines(simpleDiff)

	// new_line=2 is an added line → should be valid with new_line only
	if err := ValidateDiffPosition(lines, 2, 0); err != nil {
		t.Errorf("added line new_line=2 should be valid: %v", err)
	}
}

// TestValidateDiffPosition_RemovedLine verifies position validation for a line
// that was removed in the diff (old_line set, new_line zero).
func TestValidateDiffPosition_RemovedLine(t *testing.T) {
	lines := ParseDiffLines(simpleDiff)

	// old_line=2 is a removed line → should be valid with old_line only
	if err := ValidateDiffPosition(lines, 0, 2); err != nil {
		t.Errorf("removed line old_line=2 should be valid: %v", err)
	}
}

// TestValidateDiffPosition_ContextLine verifies position validation for an
// unchanged context line where both old_line and new_line are set.
func TestValidateDiffPosition_ContextLine(t *testing.T) {
	lines := ParseDiffLines(simpleDiff)

	// old_line=1, new_line=1 is a context line → valid with both set
	if err := ValidateDiffPosition(lines, 1, 1); err != nil {
		t.Errorf("context line (1,1) should be valid: %v", err)
	}
}

// TestValidateDiffPosition_ContextLineMissingOldLine verifies that validation
// fails when a context line has new_line but is missing old_line.
func TestValidateDiffPosition_ContextLineMissingOldLine(t *testing.T) {
	lines := ParseDiffLines(simpleDiff)

	// new_line=4 is context line (old=3, new=4) but old_line not set
	err := ValidateDiffPosition(lines, 4, 0)
	if err == nil {
		t.Fatal("context line with only new_line should return error")
	}
	// Error should mention the correct old_line
	if got := err.Error(); !contains(got, "old_line=3") {
		t.Errorf("error should suggest old_line=3, got: %s", got)
	}
}

// TestValidateDiffPosition_ContextLineMissingNewLine verifies that validation
// fails when a context line has old_line but is missing new_line.
func TestValidateDiffPosition_ContextLineMissingNewLine(t *testing.T) {
	lines := ParseDiffLines(simpleDiff)

	// old_line=3 is context line (old=3, new=4) but new_line not set
	err := ValidateDiffPosition(lines, 0, 3)
	if err == nil {
		t.Fatal("context line with only old_line should return error")
	}
	if got := err.Error(); !contains(got, "new_line=4") {
		t.Errorf("error should suggest new_line=4, got: %s", got)
	}
}

// TestValidateDiffPosition_OutsideDiff verifies that validation fails when
// the specified line numbers fall outside the parsed diff range.
func TestValidateDiffPosition_OutsideDiff(t *testing.T) {
	lines := ParseDiffLines(simpleDiff)

	// new_line=100 is not in the diff at all
	err := ValidateDiffPosition(lines, 100, 0)
	if err == nil {
		t.Fatal("line outside diff should return error")
	}
	if got := err.Error(); !contains(got, "outside the diff range") {
		t.Errorf("error should mention 'outside the diff range', got: %s", got)
	}
}

// TestValidateDiffPosition_OutsideDiff_NamesTheValidRanges verifies that the
// ranges the refusal reports are the ones the diff really covers, on both
// sides.
//
// That message is all the caller has to correct itself with: it is told to
// move the comment onto a line the diff shows, so a range computed from the
// wrong lines, left at zero, or collapsed onto a single line sends the next
// call somewhere that is refused again.
func TestValidateDiffPosition_OutsideDiff_NamesTheValidRanges(t *testing.T) {
	// simpleDiff covers new lines 1-6 and old lines 1-5.
	err := ValidateDiffPosition(ParseDiffLines(simpleDiff), 100, 200)
	if err == nil {
		t.Fatal("ValidateDiffPosition() error = nil, want a refusal for a position outside the diff")
	}
	msg := err.Error()
	if !contains(msg, "new_line 100 and old_line 200") {
		t.Errorf("error = %q, want it to name the position asked for", msg)
	}
	if !contains(msg, "valid new_line: 1-6") {
		t.Errorf("error = %q, want the new-side range 1-6 of the diff", msg)
	}
	if !contains(msg, "valid old_line: 1-5") {
		t.Errorf("error = %q, want the old-side range 1-5 of the diff", msg)
	}
}

// TestValidateDiffPosition_BothSet_RequiresAnUnchangedLine verifies every part
// of the rule GitLab applies to a position that carries both line numbers: it
// must name a line the diff leaves unchanged, and that line's two numbers must
// both be the ones asked for.
//
// Accepting a position that fails either half is worse than refusing it here:
// the handler would send it to GitLab, which rejects the note with a message
// about the diff rather than about the position, and the comment is never
// published.
func TestValidateDiffPosition_BothSet_RequiresAnUnchangedLine(t *testing.T) {
	t.Run("a changed line is refused however its numbers line up", func(t *testing.T) {
		lines := []DiffLine{{OldLine: 5, NewLine: 5, Type: LineAdded}}
		if err := ValidateDiffPosition(lines, 5, 5); err == nil {
			t.Error("ValidateDiffPosition() error = nil, want a refusal: the line both numbers name was added, not left unchanged")
		}
	})
	t.Run("an old_line that matches does not carry a new_line that does not", func(t *testing.T) {
		// old_line 2 is the removed line of simpleDiff; new_line 99 is in no hunk.
		if err := ValidateDiffPosition(ParseDiffLines(simpleDiff), 99, 2); err == nil {
			t.Error("ValidateDiffPosition() error = nil, want a refusal: new_line 99 is outside the diff")
		}
	})
}

// TestValidateDiffPosition_NoLines verifies that validation fails when the
// parsed diff contains no lines.
func TestValidateDiffPosition_NoLines(t *testing.T) {
	err := ValidateDiffPosition(nil, 1, 0)
	if err == nil {
		t.Fatal("nil diff lines should return error")
	}
}

// TestValidateDiffPosition_ZeroBoth verifies that validation fails when both
// old_line and new_line are zero, which is an invalid position.
func TestValidateDiffPosition_ZeroBoth(t *testing.T) {
	lines := ParseDiffLines(simpleDiff)
	err := ValidateDiffPosition(lines, 0, 0)
	if err == nil {
		t.Fatal("both lines zero should return error")
	}
}

// TestValidateDiffPosition_RealWorldScenario reproduces the MR !2439 bug where
// line 349 (context) was specified with only new_line, causing a silent publish failure.
func TestValidateDiffPosition_RealWorldScenario(t *testing.T) {
	lines := ParseDiffLines(sampleDiff)

	// new_line=9 is an added line in the sample diff → should be valid
	// (corresponds to the first "+" line: "    if ( AEMO_COMMS...")
	// In the sample diff, new line 9 is the first added line
	var firstAdded int
	for _, l := range lines {
		if l.Type == LineAdded {
			firstAdded = l.NewLine
			break
		}
	}
	if err := ValidateDiffPosition(lines, firstAdded, 0); err != nil {
		t.Errorf("first added line (new_line=%d) should be valid: %v", firstAdded, err)
	}

	// Find a context line and try with only new_line → should fail with helpful error
	for _, l := range lines {
		if l.Type == LineContext {
			err := ValidateDiffPosition(lines, l.NewLine, 0)
			if err == nil {
				t.Errorf("context line new_line=%d with missing old_line should fail", l.NewLine)
			}
			break
		}
	}
}

// TestParseHunkHeader_Standard verifies parsing of a standard @@ hunk header
// with both old and new line counts.
func TestParseHunkHeader_Standard(t *testing.T) {
	old, newLine := parseHunkHeader("@@ -10,5 +20,8 @@ func foo()")
	if old != 10 {
		t.Errorf("old = %d, want 10", old)
	}
	if newLine != 20 {
		t.Errorf("new = %d, want 20", newLine)
	}
}

// TestParseHunkHeader_NoCount verifies parsing of a hunk header where the
// line count is omitted, defaulting to 1.
func TestParseHunkHeader_NoCount(t *testing.T) {
	old, newLine := parseHunkHeader("@@ -1 +1 @@")
	if old != 1 {
		t.Errorf("old = %d, want 1", old)
	}
	if newLine != 1 {
		t.Errorf("new = %d, want 1", newLine)
	}
}

// TestParseHunkHeader_Invalid verifies that an invalid hunk header string
// returns zero values.
func TestParseHunkHeader_Invalid(t *testing.T) {
	old, newLine := parseHunkHeader("not a hunk header")
	if old != 0 || newLine != 0 {
		t.Errorf("invalid header should return (0,0), got (%d,%d)", old, newLine)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

// TestParseDiffLines_BeforeFirstHunk verifies the parser ignores diff metadata
// lines (and a trailing newline) that appear before the first @@ hunk header.
func TestParseDiffLines_BeforeFirstHunk(t *testing.T) {
	diff := "diff --git a/foo b/foo\nindex 0000..1111 100644\n--- a/foo\n+++ b/foo\n@@ -1,2 +1,2 @@\n line1\n-line2\n+line2_new\n"

	lines := ParseDiffLines(diff)
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3 (skipping pre-hunk metadata)", len(lines))
	}
	if lines[0].OldLine != 1 || lines[0].NewLine != 1 {
		t.Errorf("line[0] = %+v, want context (1,1)", lines[0])
	}
	if lines[1].Type != LineRemoved || lines[1].OldLine != 2 {
		t.Errorf("line[1] = %+v, want removed (2,0)", lines[1])
	}
	if lines[2].Type != LineAdded || lines[2].NewLine != 2 {
		t.Errorf("line[2] = %+v, want added (0,2)", lines[2])
	}
}

// TestValidateDiffLinePosition_DefaultBranch exercises the default branch, the
// one [ValidateDiffPosition] refuses before the loop: neither line number set.
//
// The fixture is a context line with both line numbers zero, which is the only
// one that can tell the default branch from the two single-line branches: a row
// whose NewLine is 5 matches nothing whatever branch runs, so an earlier version
// of this test passed even when the wrong branch was taken. A zero-line context
// row would be claimed by either single-line validator, which is exactly what a
// widened case expression would do.
func TestValidateDiffLinePosition_DefaultBranch(t *testing.T) {
	dl := DiffLine{Type: LineContext}
	matched, err := validateDiffLinePosition(dl, 0, 0)
	if matched || err != nil {
		t.Errorf("validateDiffLinePosition(0,0) = (%v, %v), want (false, nil)", matched, err)
	}
}

// TestBuildPositionError_BothLineNumbers covers the "and" branch of
// buildPositionError that fires when both newLine and oldLine are set.
func TestBuildPositionError_BothLineNumbers(t *testing.T) {
	diffLines := []DiffLine{
		{OldLine: 5, NewLine: 5, Type: LineContext},
		{OldLine: 0, NewLine: 10, Type: LineAdded},
		{OldLine: 15, NewLine: 0, Type: LineRemoved},
	}
	err := buildPositionError(diffLines, 100, 200)
	if err == nil {
		t.Fatal("expected error for out-of-range position")
	}
	msg := err.Error()
	if !contains(msg, "new_line 100") || !contains(msg, "old_line 200") || !contains(msg, " and ") {
		t.Errorf("error should mention both lines joined by ' and ', got: %s", msg)
	}
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
