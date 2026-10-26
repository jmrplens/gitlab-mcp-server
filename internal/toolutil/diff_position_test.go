// diff_position_test.go contains unit tests for unified diff parsing and
// position validation helpers.
package toolutil

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// positionCheckRefs identifies the synthetic snapshot all successful reads use.
var positionCheckRefs = gl.MergeRequestDiffRefs{BaseSha: "base", StartSha: "start", HeadSha: "head"}

const positionCheckMRPath = "/api/v4/projects/42/merge_requests/7"

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

// TestParseHunkHeader_TokenThatIsNotARange_PassedOver verifies that a token between the
// markers that is neither an old nor a new range is passed over rather than
// refused: a combined diff's header carries a third "@" there, and its ranges
// still number the hunk.
func TestParseHunkHeader_TokenThatIsNotARange_PassedOver(t *testing.T) {
	old, newLine := parseHunkHeader("@@@ -7,2 +9,3 @@@")
	if old != 7 || newLine != 9 {
		t.Errorf("parseHunkHeader() = (%d,%d), want (7,9)", old, newLine)
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

// TestParseHunkHeader_MalformedStart verifies that a header whose start is
// not a number is refused whole rather than read as starting at line 0: a
// hunk numbered from 0 would validate every position in it against the
// wrong line, while a refused hunk contributes no lines at all.
func TestParseHunkHeader_MalformedStart(t *testing.T) {
	tests := []struct {
		name   string
		header string
	}{
		{name: "old start", header: "@@ -x,5 +20,8 @@"},
		{name: "new start", header: "@@ -10,5 +y,8 @@"},
		{name: "empty start", header: "@@ -,5 +20,8 @@"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old, newLine := parseHunkHeader(tt.header)
			if old != 0 || newLine != 0 {
				t.Errorf("parseHunkHeader(%q) = (%d,%d), want (0,0)", tt.header, old, newLine)
			}
			lines := ParseDiffLines(tt.header + "\n+added\n context\n")
			if len(lines) != 0 {
				t.Errorf("ParseDiffLines after a malformed header numbered %d lines, want none", len(lines))
			}
		})
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

// TestBuildPositionError_DescendingLinesAndOldLineOnly verifies the ranges the
// refusal names are the true minimum and maximum whatever order the lines come
// in, and that a position giving only old_line names that line alone. The
// lines descend, so a range that kept the first line seen as its minimum, or
// the last as its maximum, would name 10-10 or 5-5.
func TestBuildPositionError_DescendingLinesAndOldLineOnly(t *testing.T) {
	diffLines := []DiffLine{
		{OldLine: 12, NewLine: 10, Type: LineContext},
		{OldLine: 7, NewLine: 5, Type: LineContext},
	}
	msg := buildPositionError(diffLines, 0, 99).Error()
	for _, want := range []string{"position (old_line 99)", "valid new_line: 5-10", "valid old_line: 7-12"} {
		t.Run(want, func(t *testing.T) {
			if !contains(msg, want) {
				t.Errorf("error = %q, want it to contain %q", msg, want)
			}
		})
	}
}

// TestValidateDiffPosition_BothSet_OldLineMustMatchToo verifies a context line
// matching the new line alone is not the position asked for when the old line
// given differs from its own.
func TestValidateDiffPosition_BothSet_OldLineMustMatchToo(t *testing.T) {
	diffLines := []DiffLine{{OldLine: 4, NewLine: 5, Type: LineContext}}
	err := ValidateDiffPosition(diffLines, 5, 3)
	if err == nil || !contains(err.Error(), "outside the diff range") {
		t.Errorf("ValidateDiffPosition(5, 3) = %v, want the outside-the-range refusal", err)
	}
}

// TestMergeRequestDiffForPositionCheck_TargetPath_StopsAtMatchingPage verifies the lookup stops at the target's first page.
func TestMergeRequestDiffForPositionCheck_TargetPath_StopsAtMatchingPage(t *testing.T) {
	cases := []struct {
		name string
		page int
		path string
	}{
		{"FirstPageNewPath", 1, "new.go"},
		{"SecondPageOldPath", 2, "old.go"},
		{"ThirdPageNewPath", 3, "new.go"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var pages []int
			snapshots := 0
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == positionCheckMRPath {
					snapshots++
					respondPositionCheckRefs(w, positionCheckRefs)
					return
				}
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				pages = append(pages, page)
				if r.URL.Query().Get("per_page") != "100" {
					t.Errorf("per_page = %q, want 100", r.URL.Query().Get("per_page"))
				}
				body := "[null,{\"new_path\":\"other.go\"}]"
				if page == tc.page {
					body = "[{\"new_path\":\"new.go\",\"old_path\":\"old.go\",\"diff\":\"patch\"}]"
				}
				testutil.RespondJSONWithPagination(w, http.StatusOK, body,
					testutil.PaginationHeaders{Page: strconv.Itoa(page), NextPage: strconv.Itoa(page + 1)})
			}))

			diff, ok := MergeRequestDiffForPositionCheck(t.Context(), client, "42", 7, tc.path, positionCheckRefs)

			if !ok || diff == nil || diff.Diff != "patch" {
				t.Fatalf("lookup = (%+v, %t), want the target patch", diff, ok)
			}
			if pages[len(pages)-1] != tc.page || snapshots != 2 {
				t.Errorf("pages = %v, snapshots = %d; want stop at %d and two snapshots", pages, snapshots, tc.page)
			}
		})
	}
}

// TestMergeRequestDiffForPositionCheck_AbsentFile_ConfirmsCompleteSnapshot verifies an absent file requires a complete stable listing.
func TestMergeRequestDiffForPositionCheck_AbsentFile_ConfirmsCompleteSnapshot(t *testing.T) {
	snapshots := 0
	pages := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == positionCheckMRPath {
			snapshots++
			respondPositionCheckRefs(w, positionCheckRefs)
			return
		}
		pages++
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))

	diff, ok := MergeRequestDiffForPositionCheck(t.Context(), client, "42", 7, "missing.go", positionCheckRefs)

	if diff != nil || !ok || snapshots != 2 || pages != 1 {
		t.Fatalf("lookup = (%+v, %t), snapshots = %d, pages = %d", diff, ok, snapshots, pages)
	}
}

// TestMergeRequestDiffForPositionCheck_PageBudget_StopsAfterHundredRequests verifies the last permitted page and the request bound.
func TestMergeRequestDiffForPositionCheck_PageBudget_StopsAfterHundredRequests(t *testing.T) {
	cases := []struct {
		name, lastBody, lastNext string
		wantOK, wantFound        bool
	}{
		{"FoundOnLastPage", "[{\"new_path\":\"target.go\"}]", "101", true, true},
		{"CompleteOnLastPage", "[]", "", true, false},
		{"MorePagesRemain", "[]", "101", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pages := 0
			client := newStablePositionCheckClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				pages++
				if pages > 100 {
					t.Error("request 101 must not be sent")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				body, next := "[]", strconv.Itoa(pages+1)
				if pages == 100 {
					body, next = tc.lastBody, tc.lastNext
				}
				testutil.RespondJSONWithPagination(w, http.StatusOK, body,
					testutil.PaginationHeaders{Page: strconv.Itoa(pages), NextPage: next})
			}))

			diff, ok := MergeRequestDiffForPositionCheck(t.Context(), client, "42", 7, "target.go", positionCheckRefs)

			if ok != tc.wantOK || (diff != nil) != tc.wantFound || pages != 100 {
				t.Errorf("lookup = (%+v, %t), pages = %d; want found=%t, ok=%t, pages=100", diff, ok, pages, tc.wantFound, tc.wantOK)
			}
		})
	}
}

// TestMergeRequestDiffForPositionCheck_InvalidExpectedRefs_SkipsRequests verifies incomplete position refs cannot start a pre-check.
func TestMergeRequestDiffForPositionCheck_InvalidExpectedRefs_SkipsRequests(t *testing.T) {
	cases := []struct {
		name string
		refs gl.MergeRequestDiffRefs
	}{
		{"BaseMissing", gl.MergeRequestDiffRefs{StartSha: "start", HeadSha: "head"}},
		{"StartMissing", gl.MergeRequestDiffRefs{BaseSha: "base", HeadSha: "head"}},
		{"HeadMissing", gl.MergeRequestDiffRefs{BaseSha: "base", StartSha: "start"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

			diff, ok := MergeRequestDiffForPositionCheck(t.Context(), client, "42", 7, "target.go", tc.refs)

			if diff != nil || ok {
				t.Fatalf("lookup = (%+v, %t), want (nil, false)", diff, ok)
			}
		})
	}
}

// TestMergeRequestDiffForPositionCheck_ChangedSnapshot_DiscardsResult verifies changes to any diff ref discard the listing evidence.
func TestMergeRequestDiffForPositionCheck_ChangedSnapshot_DiscardsResult(t *testing.T) {
	cases := []struct {
		name, pageBody string
		refs           gl.MergeRequestDiffRefs
	}{
		{"BaseChanged", "[{\"new_path\":\"target.go\"}]", gl.MergeRequestDiffRefs{BaseSha: "changed", StartSha: "start", HeadSha: "head"}},
		{"StartChanged", "[{\"new_path\":\"target.go\"}]", gl.MergeRequestDiffRefs{BaseSha: "base", StartSha: "changed", HeadSha: "head"}},
		{"HeadChanged", "[{\"new_path\":\"target.go\"}]", gl.MergeRequestDiffRefs{BaseSha: "base", StartSha: "start", HeadSha: "changed"}},
		{"AbsentFileHeadChanged", "[]", gl.MergeRequestDiffRefs{BaseSha: "base", StartSha: "start", HeadSha: "changed"}},
		{"RefsMissing", "[]", gl.MergeRequestDiffRefs{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshots := 0
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == positionCheckMRPath {
					snapshots++
					refs := positionCheckRefs
					if snapshots == 2 {
						refs = tc.refs
					}
					respondPositionCheckRefs(w, refs)
					return
				}
				testutil.RespondJSON(w, http.StatusOK, tc.pageBody)
			}))

			diff, ok := MergeRequestDiffForPositionCheck(t.Context(), client, "42", 7, "target.go", positionCheckRefs)

			if diff != nil || ok || snapshots != 2 {
				t.Fatalf("lookup = (%+v, %t), snapshots = %d; want (nil, false), 2", diff, ok, snapshots)
			}
		})
	}
}

// TestMergeRequestDiffForPositionCheck_UnconfirmedInitialSnapshot_SkipsDiffs verifies an unreadable or mismatched initial snapshot skips the listing.
func TestMergeRequestDiffForPositionCheck_UnconfirmedInitialSnapshot_SkipsDiffs(t *testing.T) {
	cases := []struct {
		name, body string
		status     int
	}{
		{"PermissionRefused", "{\"message\":\"Forbidden\"}", http.StatusForbidden},
		{"DifferentRefs", "{\"diff_refs\":{\"base_sha\":\"other\",\"start_sha\":\"start\",\"head_sha\":\"head\"}}", http.StatusOK},
		{"MissingRefs", "{}", http.StatusOK},
		{"NullSnapshot", "null", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != positionCheckMRPath {
					t.Errorf("unexpected diff request %s", r.URL.Path)
				}
				testutil.RespondJSON(w, tc.status, tc.body)
			}))

			diff, ok := MergeRequestDiffForPositionCheck(t.Context(), client, "42", 7, "target.go", positionCheckRefs)

			if diff != nil || ok || calls != 1 {
				t.Fatalf("lookup = (%+v, %t), calls = %d; want (nil, false), 1", diff, ok, calls)
			}
		})
	}
}

// TestMergeRequestDiffForPositionCheck_FinalSnapshotError_DiscardsMatchingFile verifies an unreadable final snapshot discards a match.
func TestMergeRequestDiffForPositionCheck_FinalSnapshotError_DiscardsMatchingFile(t *testing.T) {
	snapshots := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == positionCheckMRPath {
			snapshots++
			if snapshots == 1 {
				respondPositionCheckRefs(w, positionCheckRefs)
				return
			}
			w.WriteHeader(http.StatusForbidden)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, "[{\"new_path\":\"target.go\",\"diff\":\"patch\"}]")
	}))

	diff, ok := MergeRequestDiffForPositionCheck(t.Context(), client, "42", 7, "target.go", positionCheckRefs)

	if diff != nil || ok || snapshots != 2 {
		t.Fatalf("lookup = (%+v, %t), snapshots = %d; want (nil, false), 2", diff, ok, snapshots)
	}
}

// TestMergeRequestDiffForPositionCheck_LaterPageFails_DiscardsPartialEvidence verifies a late listing error cannot justify a local refusal.
func TestMergeRequestDiffForPositionCheck_LaterPageFails_DiscardsPartialEvidence(t *testing.T) {
	pages := 0
	client := newStablePositionCheckClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pages++
		if pages == 1 {
			testutil.RespondJSONWithPagination(w, http.StatusOK, "[{\"new_path\":\"other.go\"}]",
				testutil.PaginationHeaders{Page: "1", NextPage: "2"})
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))

	diff, ok := MergeRequestDiffForPositionCheck(t.Context(), client, "42", 7, "target.go", positionCheckRefs)

	if diff != nil || ok || pages != 2 {
		t.Fatalf("lookup = (%+v, %t), pages = %d; want (nil, false), 2", diff, ok, pages)
	}
}

// TestMergeRequestDiffForPositionCheck_NonAdvancingPage_SkipsCheck verifies repeated and backwards continuations abandon the pre-check.
func TestMergeRequestDiffForPositionCheck_NonAdvancingPage_SkipsCheck(t *testing.T) {
	cases := []struct{ name, next string }{{"Repeated", "3"}, {"Backwards", "2"}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pages := 0
			client := newStablePositionCheckClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				pages++
				next := "3"
				if r.URL.Query().Get("page") == "3" {
					next = tc.next
				}
				testutil.RespondJSONWithPagination(w, http.StatusOK, "[]",
					testutil.PaginationHeaders{Page: r.URL.Query().Get("page"), NextPage: next})
			}))

			diff, ok := MergeRequestDiffForPositionCheck(t.Context(), client, "42", 7, "target.go", positionCheckRefs)

			if diff != nil || ok || pages != 2 {
				t.Fatalf("lookup = (%+v, %t), pages = %d; want (nil, false), 2", diff, ok, pages)
			}
		})
	}
}

// TestMergeRequestDiffForPositionCheck_CancelledContext_SendsNoRequests verifies an already-cancelled lookup sends no request.
func TestMergeRequestDiffForPositionCheck_CancelledContext_SendsNoRequests(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	diff, ok := MergeRequestDiffForPositionCheck(ctx, client, "42", 7, "target.go", positionCheckRefs)

	if diff != nil || ok {
		t.Fatalf("lookup = (%+v, %t), want (nil, false)", diff, ok)
	}
}

// TestMergeRequestDiffForPositionCheck_CancelledNextPage_DiscardsPartialEvidence verifies cancellation reaches page two before evidence is discarded.
func TestMergeRequestDiffForPositionCheck_CancelledNextPage_DiscardsPartialEvidence(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var pages atomic.Int64
	var heldOut atomic.Bool
	finished := make(chan struct{})
	client := newStablePositionCheckClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := pages.Add(1)
		if page == 1 {
			testutil.RespondJSONWithPagination(w, http.StatusOK, "[]",
				testutil.PaginationHeaders{Page: "1", NextPage: "2"})
			return
		}
		if page != 2 {
			http.Error(w, "unexpected later page", http.StatusBadRequest)
			return
		}
		defer close(finished)
		cancel()
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
			heldOut.Store(true)
			http.Error(w, "page 2 outlived the cancelled context", http.StatusInternalServerError)
		}
	}))

	diff, ok := MergeRequestDiffForPositionCheck(ctx, client, "42", 7, "target.go", positionCheckRefs)

	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("page 2 handler did not finish after cancellation")
	}
	if diff != nil || ok || pages.Load() != 2 || heldOut.Load() {
		t.Fatalf("lookup = (%+v, %t), pages = %d, held out = %t; want (nil, false), 2, false", diff, ok, pages.Load(), heldOut.Load())
	}
}

// TestMergeRequestDiffForPositionCheck_CancelledFinalSnapshot_DiscardsMatchingFile verifies cancellation reaches the final snapshot after a matching page.
func TestMergeRequestDiffForPositionCheck_CancelledFinalSnapshot_DiscardsMatchingFile(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var snapshots atomic.Int64
	var pages atomic.Int64
	var heldOut atomic.Bool
	finished := make(chan struct{})
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != positionCheckMRPath {
			pages.Add(1)
			testutil.RespondJSON(w, http.StatusOK, "[{\"new_path\":\"target.go\"}]")
			return
		}
		snapshot := snapshots.Add(1)
		if snapshot == 1 {
			respondPositionCheckRefs(w, positionCheckRefs)
			return
		}
		if snapshot != 2 {
			http.Error(w, "unexpected later snapshot", http.StatusBadRequest)
			return
		}
		defer close(finished)
		cancel()
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
			heldOut.Store(true)
			http.Error(w, "final snapshot outlived the cancelled context", http.StatusInternalServerError)
		}
	}))

	diff, ok := MergeRequestDiffForPositionCheck(ctx, client, "42", 7, "target.go", positionCheckRefs)

	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("final snapshot handler did not finish after cancellation")
	}
	if diff != nil || ok || snapshots.Load() != 2 || pages.Load() != 1 || heldOut.Load() {
		t.Fatalf("lookup = (%+v, %t), snapshots = %d, pages = %d, held out = %t; want (nil, false), 2, 1, false", diff, ok, snapshots.Load(), pages.Load(), heldOut.Load())
	}
}

// newStablePositionCheckClient supplies only the mechanical snapshot endpoint;
// each test owns the diff endpoint and its behavioral assertions.
func newStablePositionCheckClient(t *testing.T, diffs http.Handler) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == positionCheckMRPath {
			respondPositionCheckRefs(w, positionCheckRefs)
			return
		}
		diffs.ServeHTTP(w, r)
	}))
}

// respondPositionCheckRefs writes the synthetic snapshot using GitLab's shape.
func respondPositionCheckRefs(w http.ResponseWriter, refs gl.MergeRequestDiffRefs) {
	body, err := json.Marshal(struct {
		Refs gl.MergeRequestDiffRefs `json:"diff_refs"`
	}{refs})
	if err != nil {
		http.Error(w, fmt.Sprintf("encode fixture: %v", err), http.StatusInternalServerError)
		return
	}
	testutil.RespondJSON(w, http.StatusOK, string(body))
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
