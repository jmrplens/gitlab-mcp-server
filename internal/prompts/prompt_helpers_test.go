// prompt_helpers_unit_test.go contains unit tests for prompt helper functions.
package prompts

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	// fmtExtractProjectPath identifies the fmt extract project path constant used by this package.
	fmtExtractProjectPath = "extractProjectPath() = %q, want %q"
	// testGroupAlpha identifies the test group alpha constant used by this package.
	testGroupAlpha = "group/alpha"
	// testGroupBeta identifies the test group beta constant used by this package.
	testGroupBeta = "group/beta"
	// fmtMRAge identifies the fmt MR age constant used by this package.
	fmtMRAge = "mrAge() = %q, want %q"
)

// TestParseDays_ValidInput covers ParseDays with table-driven subtests for valid input.
func TestParseDays_ValidInput(t *testing.T) {
	tests := []struct {
		input    string
		defVal   int
		expected int
	}{
		{"7", 14, 7},
		{"30", 7, 30},
		{"1", 7, 1},
		{"365", 7, 365},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseDays(tt.input, tt.defVal)
			if got != tt.expected {
				t.Errorf("parseDays(%q, %d) = %d, want %d", tt.input, tt.defVal, got, tt.expected)
			}
		})
	}
}

// TestParseDays_InvalidInput covers ParseDays with table-driven subtests for invalid input.
func TestParseDays_InvalidInput(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		defVal int
	}{
		{"empty string", "", 7},
		{"non-numeric", "abc", 14},
		{"negative", "-5", 7},
		{"zero", "0", 7},
		{"float", "3.5", 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDays(tt.input, tt.defVal)
			if got != tt.defVal {
				t.Errorf("parseDays(%q, %d) = %d, want default %d", tt.input, tt.defVal, got, tt.defVal)
			}
		})
	}
}

// TestSinceDate_ReturnsUTCPast verifies SinceDate returns utc past.
func TestSinceDate_ReturnsUTCPast(t *testing.T) {
	before := time.Now().UTC().AddDate(0, 0, -7).Truncate(24 * time.Hour)
	result := sinceDate(7)
	after := time.Now().UTC().AddDate(0, 0, -7).Truncate(24 * time.Hour)

	if result.Before(before) || result.After(after) {
		t.Errorf("sinceDate(7) = %v, expected between %v and %v", result, before, after)
	}
	if result.Location() != time.UTC {
		t.Errorf("sinceDate(7) returned non-UTC time: %v", result.Location())
	}
}

// TestExtractProjectPath_FromReferences verifies ExtractProjectPath when from references.
func TestExtractProjectPath_FromReferences(t *testing.T) {
	mr := &gl.BasicMergeRequest{
		References: &gl.IssueReferences{Full: "group/project!42"},
		ProjectID:  123,
	}
	got := extractProjectPath(mr)
	if got != "group/project" {
		t.Errorf(fmtExtractProjectPath, got, "group/project")
	}
}

// TestExtractProjectPath_FromWebURL verifies ExtractProjectPath when from web URL.
func TestExtractProjectPath_FromWebURL(t *testing.T) {
	mr := &gl.BasicMergeRequest{
		WebURL:    "https://gitlab.example.com/team/backend/-/merge_requests/99",
		ProjectID: 456,
	}
	got := extractProjectPath(mr)
	if got != "team/backend" {
		t.Errorf(fmtExtractProjectPath, got, "team/backend")
	}
}

// TestExtractProjectPath_FallbackToProjectID verifies ExtractProjectPath when fallback to project ID.
func TestExtractProjectPath_FallbackToProjectID(t *testing.T) {
	mr := &gl.BasicMergeRequest{ProjectID: 789}
	got := extractProjectPath(mr)
	if got != "project-789" {
		t.Errorf(fmtExtractProjectPath, got, "project-789")
	}
}

// TestExtractIssueProjectPath_FromReferences verifies ExtractIssueProjectPath when from references.
func TestExtractIssueProjectPath_FromReferences(t *testing.T) {
	issue := &gl.Issue{
		References: &gl.IssueReferences{Full: "team/frontend#15"},
		ProjectID:  100,
	}
	got := extractIssueProjectPath(issue)
	if got != "team/frontend" {
		t.Errorf("extractIssueProjectPath() = %q, want %q", got, "team/frontend")
	}
}

// TestGroupMRsByProject_MultipleProjects verifies GroupMRsByProject projects for multiple.
func TestGroupMRsByProject_MultipleProjects(t *testing.T) {
	mrs := []*gl.BasicMergeRequest{
		{IID: 1, References: &gl.IssueReferences{Full: "group/alpha!1"}},
		{IID: 2, References: &gl.IssueReferences{Full: "group/alpha!2"}},
		{IID: 3, References: &gl.IssueReferences{Full: "group/beta!3"}},
		{IID: 4, References: &gl.IssueReferences{Full: "team/gamma!4"}},
		{IID: 5, References: &gl.IssueReferences{Full: "group/beta!5"}},
	}

	grouped := groupMRsByProject(mrs)

	if len(grouped) != 3 {
		t.Fatalf("expected 3 projects, got %d", len(grouped))
	}
	if len(grouped[testGroupAlpha]) != 2 {
		t.Errorf("group/alpha: expected 2 MRs, got %d", len(grouped[testGroupAlpha]))
	}
	if len(grouped[testGroupBeta]) != 2 {
		t.Errorf("group/beta: expected 2 MRs, got %d", len(grouped[testGroupBeta]))
	}
	if len(grouped["team/gamma"]) != 1 {
		t.Errorf("team/gamma: expected 1 MR, got %d", len(grouped["team/gamma"]))
	}
}

// TestGroupMRsByProject_EmptyInput verifies GroupMRsByProject when empty input.
func TestGroupMRsByProject_EmptyInput(t *testing.T) {
	grouped := groupMRsByProject(nil)
	if len(grouped) != 0 {
		t.Errorf("expected empty map, got %d entries", len(grouped))
	}
}

// TestGroupIssuesByProject_MultipleProjects verifies GroupIssuesByProject projects for multiple.
func TestGroupIssuesByProject_MultipleProjects(t *testing.T) {
	issues := []*gl.Issue{
		{IID: 1, References: &gl.IssueReferences{Full: "group/alpha#1"}},
		{IID: 2, References: &gl.IssueReferences{Full: "group/alpha#2"}},
		{IID: 3, References: &gl.IssueReferences{Full: "group/beta#3"}},
	}

	grouped := groupIssuesByProject(issues)

	if len(grouped) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(grouped))
	}
	if len(grouped[testGroupAlpha]) != 2 {
		t.Errorf("group/alpha: expected 2 issues, got %d", len(grouped[testGroupAlpha]))
	}
	if len(grouped[testGroupBeta]) != 1 {
		t.Errorf("group/beta: expected 1 issue, got %d", len(grouped[testGroupBeta]))
	}
}

// TestMRAge_Days verifies MRAge when days.
func TestMRAge_Days(t *testing.T) {
	created := time.Now().Add(-3 * 24 * time.Hour)
	mr := &gl.BasicMergeRequest{CreatedAt: &created}
	got := mrAge(mr)
	if got != "3d" {
		t.Errorf(fmtMRAge, got, "3d")
	}
}

// TestMRAge_Weeks verifies MRAge when weeks.
func TestMRAge_Weeks(t *testing.T) {
	created := time.Now().Add(-14 * 24 * time.Hour)
	mr := &gl.BasicMergeRequest{CreatedAt: &created}
	got := mrAge(mr)
	if got != "2w" {
		t.Errorf(fmtMRAge, got, "2w")
	}
}

// TestMRAge_NilCreatedAt verifies MRAge when nil created at.
func TestMRAge_NilCreatedAt(t *testing.T) {
	mr := &gl.BasicMergeRequest{}
	got := mrAge(mr)
	if got != "?" {
		t.Errorf(fmtMRAge, got, "?")
	}
}

// TestIssueAge_Days verifies IssueAge when days.
func TestIssueAge_Days(t *testing.T) {
	created := time.Now().Add(-5 * 24 * time.Hour)
	issue := &gl.Issue{CreatedAt: &created}
	got := issueAge(issue)
	if got != "5d" {
		t.Errorf("issueAge() = %q, want %q", got, "5d")
	}
}

// TestFormatAge_AllRanges covers FormatAge with table-driven subtests for all ranges.
func TestFormatAge_AllRanges(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"less than a day", 12 * time.Hour, "<1d"},
		{"3 days", 3 * 24 * time.Hour, "3d"},
		{"2 weeks", 14 * 24 * time.Hour, "2w"},
		{"2 months", 60 * 24 * time.Hour, "2mo"},
		{"1 year", 400 * 24 * time.Hour, "1y"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatAge(tt.duration)
			if got != tt.expected {
				t.Errorf("formatAge(%v) = %q, want %q", tt.duration, got, tt.expected)
			}
		})
	}
}

// TestPipelineEmoji_AllStatuses covers PipelineEmoji with table-driven subtests for all statuses.
func TestPipelineEmoji_AllStatuses(t *testing.T) {
	tests := []struct {
		status   string
		expected string
	}{
		{"success", "✅"},
		{"failed", "❌"},
		{"running", "⏳"},
		{"pending", "⏳"},
		{"canceled", "🚫"},
		{"canceled", "🚫"},
		{"skipped", "⏭️"},
		{"unknown", "⚪"},
		{"", "⚪"},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := pipelineEmoji(tt.status)
			if got != tt.expected {
				t.Errorf("pipelineEmoji(%q) = %q, want %q", tt.status, got, tt.expected)
			}
		})
	}
}

// TestWriteMRTable_Format verifies WriteMRTable when format.
func TestWriteMRTable_Format(t *testing.T) {
	created := time.Now().Add(-2 * 24 * time.Hour)
	mrs := []*gl.BasicMergeRequest{
		{
			IID:          42,
			Title:        "Fix login",
			Author:       &gl.BasicUser{Username: "alice"},
			SourceBranch: "feature/fix",
			TargetBranch: "develop",
			CreatedAt:    &created,
		},
	}
	var b strings.Builder
	writeMRTable(&b, mrs)
	output := b.String()

	if !strings.Contains(output, "| MR | Title |") {
		t.Error("expected table header")
	}
	if !strings.Contains(output, "!42") {
		t.Error("expected MR IID !42")
	}
	if !strings.Contains(output, "Fix login") {
		t.Error("expected MR title")
	}
	if !strings.Contains(output, "@alice") {
		t.Error("expected author @alice")
	}
	if !strings.Contains(output, "feature/fix -> develop") {
		t.Error("expected branch info")
	}
}

// TestWriteMRTable_EmptyList verifies WriteMRTable when empty list.
func TestWriteMRTable_EmptyList(t *testing.T) {
	var b strings.Builder
	writeMRTable(&b, nil)
	if !strings.Contains(b.String(), "No merge requests found") {
		t.Error("expected empty message")
	}
}

// TestWriteIssueTable_Format verifies WriteIssueTable when format.
func TestWriteIssueTable_Format(t *testing.T) {
	created := time.Now().Add(-5 * 24 * time.Hour)
	dueDate := gl.ISOTime(time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC))
	issues := []*gl.Issue{
		{
			IID:       15,
			Title:     "Fix timeout",
			Labels:    gl.Labels{"bug", "backend"},
			Milestone: &gl.Milestone{Title: "v2.1"},
			CreatedAt: &created,
			DueDate:   &dueDate,
		},
	}
	var b strings.Builder
	writeIssueTable(&b, issues)
	output := b.String()

	if !strings.Contains(output, "| Issue | Title |") {
		t.Error("expected table header")
	}
	if !strings.Contains(output, "#15") {
		t.Error("expected issue IID #15")
	}
	if !strings.Contains(output, "bug, backend") {
		t.Error("expected labels")
	}
	if !strings.Contains(output, "v2.1") {
		t.Error("expected milestone")
	}
	if !strings.Contains(output, "2026-03-15") {
		t.Error("expected due date")
	}
}

// TestWriteIssueTable_EmptyList verifies WriteIssueTable when empty list.
func TestWriteIssueTable_EmptyList(t *testing.T) {
	var b strings.Builder
	writeIssueTable(&b, nil)
	if !strings.Contains(b.String(), "No issues found") {
		t.Error("expected empty message")
	}
}

// TestMRStatusDraft_WithConflicts verifies MRStatusDraft when with conflicts.
func TestMRStatusDraft_WithConflicts(t *testing.T) {
	mr := &gl.BasicMergeRequest{Draft: true, HasConflicts: true, DetailedMergeStatus: "mergeable"}
	got := mrStatus(mr)
	if !strings.Contains(got, "draft") {
		t.Errorf("expected 'draft' in status, got %q", got)
	}
	if !strings.Contains(got, "conflicts") {
		t.Errorf("expected 'conflicts' in status, got %q", got)
	}
}

// TestMRStatus_NoFlags verifies MRStatus flags for no.
func TestMRStatus_NoFlags(t *testing.T) {
	mr := &gl.BasicMergeRequest{}
	got := mrStatus(mr)
	if got != "-" {
		t.Errorf("expected dash, got %q", got)
	}
}

// TestMergeDuration verifies MergeDuration.
func TestMergeDuration(t *testing.T) {
	created := time.Now().Add(-48 * time.Hour)
	merged := time.Now()
	mr := &gl.BasicMergeRequest{CreatedAt: &created, MergedAt: &merged}
	d := mergeDuration(mr)
	if d < 47*time.Hour || d > 49*time.Hour {
		t.Errorf("mergeDuration() = %v, expected ~48h", d)
	}
}

// TestMergeDuration_NilTimestamps verifies MergeDuration when nil timestamps.
func TestMergeDuration_NilTimestamps(t *testing.T) {
	mr := &gl.BasicMergeRequest{}
	if d := mergeDuration(mr); d != 0 {
		t.Errorf("mergeDuration() = %v, want 0", d)
	}
}

// TestFormatDuration covers FormatDuration with table-driven subtests.
func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		d        time.Duration
		expected string
	}{
		{"zero", 0, "-"},
		{"minutes", 45 * time.Minute, "45m"},
		{"hours", 3*time.Hour + 20*time.Minute, "3h 20m"},
		{"days", 50 * time.Hour, "2d 2h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.d)
			if got != tt.expected {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.expected)
			}
		})
	}
}

// TestAvgDuration verifies AvgDuration.
func TestAvgDuration(t *testing.T) {
	durations := []time.Duration{2 * time.Hour, 4 * time.Hour, 6 * time.Hour}
	avg := avgDuration(durations)
	if avg != 4*time.Hour {
		t.Errorf("avgDuration() = %v, want 4h", avg)
	}
}

// TestAvgDuration_Empty verifies AvgDuration when empty.
func TestAvgDuration_Empty(t *testing.T) {
	if avg := avgDuration(nil); avg != 0 {
		t.Errorf("avgDuration(nil) = %v, want 0", avg)
	}
}

// TestMedianDuration covers MedianDuration with table-driven subtests.
func TestMedianDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    []time.Duration
		expected time.Duration
	}{
		{"odd count", []time.Duration{1 * time.Hour, 3 * time.Hour, 5 * time.Hour}, 3 * time.Hour},
		{"even count", []time.Duration{1 * time.Hour, 2 * time.Hour, 3 * time.Hour, 4 * time.Hour}, 150 * time.Minute},
		{"empty", nil, 0},
		{"single", []time.Duration{42 * time.Minute}, 42 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := medianDuration(tt.input)
			if got != tt.expected {
				t.Errorf("medianDuration() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestProgressBar covers ProgressBar with table-driven subtests.
func TestProgressBar(t *testing.T) {
	tests := []struct {
		name     string
		done     int
		total    int
		expected string
	}{
		{"0%", 0, 10, "[░░░░░░░░░░] 0%"},
		{"50%", 5, 10, "[█████░░░░░] 50%"},
		{"100%", 10, 10, "[██████████] 100%"},
		{"zero total", 0, 0, "[░░░░░░░░░░] 0%"},
		{"80%", 16, 20, "[████████░░] 80%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := progressBar(tt.done, tt.total)
			if got != tt.expected {
				t.Errorf("progressBar(%d, %d) = %q, want %q", tt.done, tt.total, got, tt.expected)
			}
		})
	}
}

// TestDeduplicateMRs verifies DeduplicateMRs.
func TestDeduplicateMRs(t *testing.T) {
	mrs1 := []*gl.BasicMergeRequest{
		{IID: 1, ProjectID: 100},
		{IID: 2, ProjectID: 100},
	}
	mrs2 := []*gl.BasicMergeRequest{
		{IID: 2, ProjectID: 100}, // duplicate
		{IID: 3, ProjectID: 200},
	}
	result := deduplicateMRs(mrs1, mrs2)
	if len(result) != 3 {
		t.Errorf("deduplicateMRs: expected 3, got %d", len(result))
	}
}

// TestGetArgOr verifies GetArgOr.
func TestGetArgOr(t *testing.T) {
	args := map[string]string{"state": "closed", "empty": ""}
	if got := getArgOr(args, "state", "opened"); got != "closed" {
		t.Errorf("expected 'closed', got %q", got)
	}
	if got := getArgOr(args, "missing", "default"); got != "default" {
		t.Errorf("expected 'default', got %q", got)
	}
	if got := getArgOr(args, "empty", "fallback"); got != "fallback" {
		t.Errorf("expected 'fallback', got %q", got)
	}
}

// TestSortedKeys verifies SortedKeys.
func TestSortedKeys(t *testing.T) {
	m := map[string]int{"charlie": 1, "alpha": 2, "bravo": 3}
	keys := sortedKeys(m)
	expected := []string{"alpha", "bravo", "charlie"}
	if len(keys) != len(expected) {
		t.Fatalf("expected %d keys, got %d", len(expected), len(keys))
	}
	for i, k := range keys {
		if k != expected[i] {
			t.Errorf("key[%d] = %q, want %q", i, k, expected[i])
		}
	}
}

// TestExtractIssueProjectPath_FromWebURL covers the WebURL fallback branch
// when References is nil.
func TestExtractIssueProjectPath_FromWebURL(t *testing.T) {
	issue := &gl.Issue{
		WebURL:    "https://gitlab.example.com/team/frontend/-/issues/15",
		ProjectID: 100,
	}
	got := extractIssueProjectPath(issue)
	if got != "team/frontend" {
		t.Errorf("extractIssueProjectPath() = %q, want %q", got, "team/frontend")
	}
}

// TestExtractIssueProjectPath_FallbackToProjectID covers the last fallback
// when both References and WebURL are empty.
func TestExtractIssueProjectPath_FallbackToProjectID(t *testing.T) {
	issue := &gl.Issue{ProjectID: 42}
	got := extractIssueProjectPath(issue)
	if got != "project-42" {
		t.Errorf("extractIssueProjectPath() = %q, want %q", got, "project-42")
	}
}

// TestIssueAge_NilCreatedAt covers the nil CreatedAt fallback returning "?".
func TestIssueAge_NilCreatedAt(t *testing.T) {
	issue := &gl.Issue{}
	got := issueAge(issue)
	if got != "?" {
		t.Errorf("issueAge() = %q, want %q", got, "?")
	}
}

// TestReadinessLabel covers all three branches of readinessLabel, at each
// threshold and on both sides of it.
//
// The label is compared whole rather than by substring: "Ready" is a substring
// of "Not Ready", so a containment check passes the ready case on the verdict
// that means the opposite, which is the one reading of this function nobody
// would want to ship. The two thresholds are taken exactly for the reason every
// boundary here is: read one step over, five blockers become a refusal and one
// becomes nothing to report.
func TestReadinessLabel(t *testing.T) {
	tests := []struct {
		name     string
		blockers int
		want     string
	}{
		{"many_blockers_not_ready", 10, toolutil.EmojiRed + " Not Ready"},
		{"six_blockers_not_ready", 6, toolutil.EmojiRed + " Not Ready"},
		{"exactly_five_blockers_needs_attention", 5, toolutil.EmojiYellow + " Needs Attention"},
		{"few_blockers_needs_attention", 3, toolutil.EmojiYellow + " Needs Attention"},
		{"one_blocker_needs_attention", 1, toolutil.EmojiYellow + " Needs Attention"},
		{"no_blockers_ready", 0, toolutil.EmojiGreen + " Ready"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := readinessLabel(tt.blockers)
			if got != tt.want {
				t.Errorf("readinessLabel(%d) = %q, want %q", tt.blockers, got, tt.want)
			}
		})
	}
}

// TestReleaseDate covers all three branches: ReleasedAt, CreatedAt, zero.
func TestReleaseDate(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-24 * time.Hour)

	t.Run("ReleasedAt", func(t *testing.T) {
		r := &gl.Release{ReleasedAt: &now}
		got := releaseDate(r)
		if !got.Equal(now) {
			t.Errorf("got %v, want %v", got, now)
		}
	})
	t.Run("CreatedAt_fallback", func(t *testing.T) {
		r := &gl.Release{CreatedAt: &earlier}
		got := releaseDate(r)
		if !got.Equal(earlier) {
			t.Errorf("got %v, want %v", got, earlier)
		}
	})
	t.Run("zero", func(t *testing.T) {
		r := &gl.Release{}
		got := releaseDate(r)
		if !got.IsZero() {
			t.Errorf("got %v, want zero", got)
		}
	})
}

// TestSafeLen covers both branches of safeLen.
func TestSafeLen(t *testing.T) {
	if got := safeLen(5, nil); got != 5 {
		t.Errorf("safeLen(5, nil) = %d, want 5", got)
	}
	if got := safeLen(5, errors.New("fail")); got != 0 {
		t.Errorf("safeLen(5, err) = %d, want 0", got)
	}
}

// TestWriteCountRow covers both branches of writeCountRow.
func TestWriteCountRow(t *testing.T) {
	var b strings.Builder
	writeCountRow(&b, "Issues", 42, nil)
	if !strings.Contains(b.String(), "42") {
		t.Errorf("expected count 42, got %q", b.String())
	}

	b.Reset()
	writeCountRow(&b, "Issues", 0, errors.New("fail"))
	if !strings.Contains(b.String(), "N/A") {
		t.Errorf("expected N/A, got %q", b.String())
	}
}

// TestFormatBranchAccessLevel covers all branches.
func TestFormatBranchAccessLevel(t *testing.T) {
	tests := []struct {
		name string
		al   *gl.BranchAccessDescription
		want string
	}{
		{"user_id", &gl.BranchAccessDescription{UserID: 5, AccessLevel: gl.DeveloperPermissions}, "User #5"},
		{"group_id", &gl.BranchAccessDescription{GroupID: 10, AccessLevel: gl.MaintainerPermissions}, "Group #10"},
		{"level_only", &gl.BranchAccessDescription{AccessLevel: gl.MaintainerPermissions}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatBranchAccessLevel(tt.al)
			if tt.want != "" && !strings.Contains(got, tt.want) {
				t.Errorf("formatBranchAccessLevel() = %q, want containing %q", got, tt.want)
			}
		})
	}
}

// TestAccessLevelIcon covers both branches.
func TestAccessLevelIcon(t *testing.T) {
	tests := []struct {
		name  string
		input gl.AccessControlValue
	}{
		{"enabled", gl.EnabledAccessControl},
		{"disabled", gl.DisabledAccessControl},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if accessLevelIcon(tt.input) == "" {
				t.Errorf("expected non-empty icon for %s", tt.name)
			}
		})
	}
}

// TestFormatAuditDate covers both branches.
func TestFormatAuditDate(t *testing.T) {
	if got := formatAuditDate(nil); got != "-" {
		t.Errorf("formatAuditDate(nil) = %q, want %q", got, "-")
	}
	now := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)
	if got := formatAuditDate(&now); got != "2025-03-15" {
		t.Errorf("formatAuditDate() = %q, want %q", got, "2025-03-15")
	}
}

// TestWriteReleaseNotesMRs verifies the release notes MR section formatter
// covers: empty MRs, MR with nil author, MR with labels, MR with long description.
func TestWriteReleaseNotesMRs(t *testing.T) {
	t.Run("empty_mrs_produces_no_output", func(t *testing.T) {
		var b strings.Builder
		writeReleaseNotesMRs(&b, nil)
		if b.Len() != 0 {
			t.Errorf("expected empty output for nil MRs, got: %q", b.String())
		}
	})

	t.Run("mr_with_nil_author", func(t *testing.T) {
		var b strings.Builder
		mrs := []*gl.BasicMergeRequest{{IID: 10, Title: "Fix bug"}}
		writeReleaseNotesMRs(&b, mrs)
		out := b.String()
		if !strings.Contains(out, "Merge Requests (1)") {
			t.Errorf("expected heading with count, got: %s", out)
		}
		if !strings.Contains(out, "@unknown") {
			t.Errorf("expected @unknown for nil author, got: %s", out)
		}
	})

	t.Run("mr_with_author_and_labels", func(t *testing.T) {
		var b strings.Builder
		mrs := []*gl.BasicMergeRequest{{
			IID:    20,
			Title:  "Add feature",
			Author: &gl.BasicUser{Username: "alice"},
			Labels: gl.Labels{"bug", "enhancement"},
		}}
		writeReleaseNotesMRs(&b, mrs)
		out := b.String()
		if !strings.Contains(out, "@alice") {
			t.Errorf("expected author username, got: %s", out)
		}
		if !strings.Contains(out, "[bug, enhancement]") {
			t.Errorf("expected labels in output, got: %s", out)
		}
	})

	t.Run("mr_with_long_description_truncated", func(t *testing.T) {
		var b strings.Builder
		longDesc := strings.Repeat("x", 250)
		mrs := []*gl.BasicMergeRequest{{
			IID:         30,
			Title:       "Big MR",
			Author:      &gl.BasicUser{Username: "bob"},
			Description: longDesc,
		}}
		writeReleaseNotesMRs(&b, mrs)
		out := b.String()
		if !strings.Contains(out, "...") {
			t.Errorf("expected truncated description with '...', got: %s", out)
		}
	})

	t.Run("mr_with_short_description", func(t *testing.T) {
		var b strings.Builder
		mrs := []*gl.BasicMergeRequest{{
			IID:         40,
			Title:       "Small MR",
			Author:      &gl.BasicUser{Username: "carol"},
			Description: "Short desc",
		}}
		writeReleaseNotesMRs(&b, mrs)
		out := b.String()
		if !strings.Contains(out, "> Short desc") {
			t.Errorf("expected short description verbatim, got: %s", out)
		}
	})

	t.Run("mr_with_multiline_description_takes_first_line", func(t *testing.T) {
		var b strings.Builder
		mrs := []*gl.BasicMergeRequest{{
			IID:         50,
			Title:       "Multi MR",
			Author:      &gl.BasicUser{Username: "dave"},
			Description: "First line\nSecond line\nThird line",
		}}
		writeReleaseNotesMRs(&b, mrs)
		out := b.String()
		if !strings.Contains(out, "> First line") {
			t.Errorf("expected first line only, got: %s", out)
		}
		if strings.Contains(out, "Second line") {
			t.Errorf("should not contain second line, got: %s", out)
		}
	})
}

// TestWriteDailyActivityChart verifies the Mermaid chart formatter for daily
// activity data including single and multiple days.
func TestWriteDailyActivityChart(t *testing.T) {
	t.Run("single_day", func(t *testing.T) {
		var b strings.Builder
		data := []dayActivity{{date: "2025-03-15", count: 5}}
		writeDailyActivityChart(&b, data)
		out := b.String()
		if !strings.Contains(out, "03-15") {
			t.Errorf("expected date suffix in x-axis, got: %s", out)
		}
		if !strings.Contains(out, "bar [5]") {
			t.Errorf("expected bar data, got: %s", out)
		}
	})

	t.Run("multiple_days", func(t *testing.T) {
		var b strings.Builder
		data := []dayActivity{
			{date: "2025-03-14", count: 3},
			{date: "2025-03-15", count: 7},
		}
		writeDailyActivityChart(&b, data)
		out := b.String()
		if !strings.Contains(out, "03-14\", \"03-15\"") {
			t.Errorf("expected comma-separated dates, got: %s", out)
		}
		if !strings.Contains(out, "bar [3, 7]") {
			t.Errorf("expected bar values, got: %s", out)
		}
	})
}

// TestClassifyMembers verifies member classification by access level and state.
func TestClassifyMembers(t *testing.T) {
	members := []*gl.ProjectMember{
		{AccessLevel: 50, State: "active"},
		{AccessLevel: 40, State: "active"},
		{AccessLevel: 30, State: "blocked"},
		{AccessLevel: 20, State: "awaiting"},
		{AccessLevel: 10, State: "active"},
	}

	g := classifyMembers(members)
	if len(g.owners) != 1 {
		t.Errorf("owners = %d, want 1", len(g.owners))
	}
	if len(g.maintainers) != 1 {
		t.Errorf("maintainers = %d, want 1", len(g.maintainers))
	}
	if len(g.developers) != 1 {
		t.Errorf("developers = %d, want 1", len(g.developers))
	}
	if len(g.reporters) != 1 {
		t.Errorf("reporters = %d, want 1", len(g.reporters))
	}
	if len(g.guests) != 1 {
		t.Errorf("guests = %d, want 1", len(g.guests))
	}
	if len(g.blocked) != 1 {
		t.Errorf("blocked = %d, want 1", len(g.blocked))
	}
	if len(g.inactive) != 1 {
		t.Errorf("inactive = %d, want 1 (awaiting state)", len(g.inactive))
	}
}

// TestWriteMilestonesAudit verifies milestone audit formatting for zero, active,
// active-with-expired, and mixed active+closed scenarios.
func TestWriteMilestonesAudit(t *testing.T) {
	t.Run("no_milestones", func(t *testing.T) {
		var b strings.Builder
		writeMilestonesAudit(&b, nil, nil)
		if !strings.Contains(b.String(), "No milestones configured") {
			t.Errorf("expected no-milestones warning, got: %s", b.String())
		}
	})

	t.Run("active_with_due_date_and_expired", func(t *testing.T) {
		var b strings.Builder
		expired := true
		due := gl.ISOTime(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		writeMilestonesAudit(&b, []*gl.Milestone{
			{Title: "v1.0", DueDate: &due, Expired: &expired},
		}, nil)
		out := b.String()
		if !strings.Contains(out, "**Active:** 1") {
			t.Errorf("expected active count, got: %s", out)
		}
		if !strings.Contains(out, "Yes") {
			t.Errorf("expected expired indicator, got: %s", out)
		}
	})

	t.Run("active_with_due_date_not_expired", func(t *testing.T) {
		var b strings.Builder
		notExpired := false
		due := gl.ISOTime(time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))
		writeMilestonesAudit(&b, []*gl.Milestone{
			{Title: "v2.0", DueDate: &due, Expired: &notExpired},
		}, nil)
		out := b.String()
		if !strings.Contains(out, "No") {
			t.Errorf("expected 'No' for not-expired, got: %s", out)
		}
	})

	t.Run("active_without_due_date", func(t *testing.T) {
		var b strings.Builder
		writeMilestonesAudit(&b, []*gl.Milestone{
			{Title: "backlog"},
		}, nil)
		out := b.String()
		if !strings.Contains(out, "not set") {
			t.Errorf("expected 'not set' for no due date, got: %s", out)
		}
	})

	t.Run("with_closed_milestones", func(t *testing.T) {
		var b strings.Builder
		writeMilestonesAudit(&b, nil, []*gl.Milestone{{Title: "old"}})
		out := b.String()
		if !strings.Contains(out, "**Closed:** 1") {
			t.Errorf("expected closed count, got: %s", out)
		}
	})
}

// TestWriteFullAccessSection verifies the full audit access section covers
// members by access level and shared groups.
func TestWriteFullAccessSection(t *testing.T) {
	t.Run("members_with_multiple_levels", func(t *testing.T) {
		var b strings.Builder
		members := []*gl.ProjectMember{
			{AccessLevel: 50}, // Owner
			{AccessLevel: 40}, // Maintainer
			{AccessLevel: 30}, // Developer
		}
		writeFullAccessSection(&b, members, nil, nil)
		out := b.String()
		if !strings.Contains(out, "**Total members:** 3") {
			t.Errorf("expected total count, got: %s", out)
		}
		if !strings.Contains(out, "Owner") {
			t.Errorf("expected Owner level, got: %s", out)
		}
		if !strings.Contains(out, "Developer") {
			t.Errorf("expected Developer level, got: %s", out)
		}
	})

	t.Run("with_shared_groups", func(t *testing.T) {
		var b strings.Builder
		groups := []gl.ProjectSharedWithGroup{
			{GroupName: "team-a", GroupAccessLevel: 30},
		}
		writeFullAccessSection(&b, nil, groups, nil)
		out := b.String()
		if !strings.Contains(out, "team-a") {
			t.Errorf("expected group name, got: %s", out)
		}
		if !strings.Contains(out, "Shared with 1 group") {
			t.Errorf("expected shared groups heading, got: %s", out)
		}
	})

	t.Run("empty_members_and_groups", func(t *testing.T) {
		var b strings.Builder
		writeFullAccessSection(&b, nil, nil, nil)
		out := b.String()
		if !strings.Contains(out, "**Total members:** 0") {
			t.Errorf("expected zero members, got: %s", out)
		}
	})
}

// TestWriteFullLabelsSection verifies label section in full audit covers
// both with and without descriptions.
func TestWriteFullLabelsSection(t *testing.T) {
	t.Run("labels_with_missing_description", func(t *testing.T) {
		var b strings.Builder
		labels := []*gl.Label{
			{Name: "bug", Description: "Bug reports"},
			{Name: "todo", Description: ""},
		}
		writeFullLabelsSection(&b, labels, nil)
		out := b.String()
		if !strings.Contains(out, "**Total:** 2") {
			t.Errorf("expected label count, got: %s", out)
		}
		if !strings.Contains(out, "1 label(s) without description") {
			t.Errorf("expected missing description warning, got: %s", out)
		}
	})

	t.Run("labels_all_with_description", func(t *testing.T) {
		var b strings.Builder
		labels := []*gl.Label{
			{Name: "bug", Description: "Bug reports"},
		}
		writeFullLabelsSection(&b, labels, nil)
		out := b.String()
		if strings.Contains(out, "without description") {
			t.Errorf("should not warn when all have descriptions, got: %s", out)
		}
	})

	t.Run("no_labels", func(t *testing.T) {
		var b strings.Builder
		writeFullLabelsSection(&b, nil, nil)
		out := b.String()
		if !strings.Contains(out, "**Total:** 0") {
			t.Errorf("expected zero count, got: %s", out)
		}
	})
}

// TestWriteFullMilestonesSection verifies milestone section in full audit
// covers milestones with and without due dates.
func TestWriteFullMilestonesSection(t *testing.T) {
	t.Run("milestone_with_due_date", func(t *testing.T) {
		var b strings.Builder
		due := gl.ISOTime(time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC))
		writeFullMilestonesSection(&b, []*gl.Milestone{
			{Title: "v1.0", DueDate: &due},
		}, nil)
		out := b.String()
		if !strings.Contains(out, "**Active:** 1") {
			t.Errorf("expected active count, got: %s", out)
		}
		if !strings.Contains(out, "due") {
			t.Errorf("expected due date, got: %s", out)
		}
	})

	t.Run("milestone_without_due_date", func(t *testing.T) {
		var b strings.Builder
		writeFullMilestonesSection(&b, []*gl.Milestone{
			{Title: "backlog"},
		}, nil)
		out := b.String()
		if !strings.Contains(out, "no due date") {
			t.Errorf("expected 'no due date', got: %s", out)
		}
	})
}

// TestWriteFullPushRulesSection verifies push rules section in full audit
// covers nil push rules and configured rules.
func TestWriteFullPushRulesSection(t *testing.T) {
	t.Run("nil_push_rules", func(t *testing.T) {
		var b strings.Builder
		writeFullPushRulesSection(&b, nil, nil)
		out := b.String()
		if !strings.Contains(out, "Push rules not configured") {
			t.Errorf("expected nil-rules message, got: %s", out)
		}
	})

	t.Run("configured_push_rules", func(t *testing.T) {
		var b strings.Builder
		writeFullPushRulesSection(&b, &gl.ProjectPushRules{
			PreventSecrets:     true,
			MemberCheck:        false,
			CommitMessageRegex: "^(feat|fix):",
			BranchNameRegex:    "^(feature|fix)/",
			AuthorEmailRegex:   "@example.com$",
		}, nil)
		out := b.String()
		if !strings.Contains(out, "Prevent secrets") {
			t.Errorf("expected push rule rows, got: %s", out)
		}
		// The rule reaches the reader with its alternation pipe encoded, since
		// a raw one would end the cell it is written into.
		if !strings.Contains(out, "^(feat&#124;fix):") {
			t.Errorf("expected commit regex, got: %s", out)
		}
		if strings.Contains(out, "^(feat|fix):") {
			t.Errorf("commit regex reached the row with a raw pipe, got: %s", out)
		}
	})
}

// TestWriteFullBranchSection verifies branch protection section in full audit.
func TestWriteFullBranchSection(t *testing.T) {
	t.Run("no_branches", func(t *testing.T) {
		var b strings.Builder
		writeFullBranchSection(&b, nil, nil)
		out := b.String()
		if !strings.Contains(out, "**Protected branches:** 0") {
			t.Errorf("expected zero count, got: %s", out)
		}
	})

	t.Run("with_branches", func(t *testing.T) {
		var b strings.Builder
		branches := []*gl.ProtectedBranch{
			{
				Name:             "main",
				AllowForcePush:   false,
				PushAccessLevels: []*gl.BranchAccessDescription{{AccessLevel: 40}},
			},
		}
		writeFullBranchSection(&b, branches, nil)
		out := b.String()
		if !strings.Contains(out, "**Protected branches:** 1") {
			t.Errorf("expected one branch, got: %s", out)
		}
		if !strings.Contains(out, "main") {
			t.Errorf("expected branch name, got: %s", out)
		}
	})
}

// TestWriteBranchDetail verifies branch detail output including default suffix
// and unprotect access levels.
func TestWriteBranchDetail(t *testing.T) {
	t.Run("default_branch_suffix", func(t *testing.T) {
		var b strings.Builder
		writeBranchDetail(&b, &gl.ProtectedBranch{Name: "main"}, "main")
		out := b.String()
		if !strings.Contains(out, "(default)") {
			t.Errorf("expected (default) suffix, got: %s", out)
		}
	})

	t.Run("non_default_branch", func(t *testing.T) {
		var b strings.Builder
		writeBranchDetail(&b, &gl.ProtectedBranch{Name: "release"}, "main")
		out := b.String()
		if strings.Contains(out, "(default)") {
			t.Errorf("should not have (default) suffix, got: %s", out)
		}
	})

	t.Run("with_unprotect_access_levels", func(t *testing.T) {
		var b strings.Builder
		writeBranchDetail(&b, &gl.ProtectedBranch{
			Name:                  "feature",
			UnprotectAccessLevels: []*gl.BranchAccessDescription{{AccessLevel: 40}},
		}, "main")
		out := b.String()
		if !strings.Contains(out, "Unprotect access") {
			t.Errorf("expected unprotect access line, got: %s", out)
		}
	})
}

// TestWriteSharedGroups verifies shared group section formatting.
func TestWriteSharedGroups(t *testing.T) {
	t.Run("empty_groups", func(t *testing.T) {
		var b strings.Builder
		writeSharedGroups(&b, nil)
		if b.Len() != 0 {
			t.Errorf("expected empty output for nil groups, got: %q", b.String())
		}
	})

	t.Run("with_groups", func(t *testing.T) {
		var b strings.Builder
		writeSharedGroups(&b, []gl.ProjectSharedWithGroup{
			{GroupName: "team-a", GroupAccessLevel: 30},
			{GroupName: "team-b", GroupAccessLevel: 40},
		})
		out := b.String()
		if !strings.Contains(out, "team-a") {
			t.Errorf("expected group name, got: %s", out)
		}
		if !strings.Contains(out, "team-b") {
			t.Errorf("expected second group name, got: %s", out)
		}
	})
}

// TestGroupEventsByDay verifies event grouping with nil dates and sorting.
func TestGroupEventsByDay(t *testing.T) {
	t.Run("nil_createdat_skipped", func(t *testing.T) {
		events := []*gl.ContributionEvent{
			{CreatedAt: nil},
		}
		result := groupEventsByDay(events)
		if len(result) != 0 {
			t.Errorf("expected empty result for nil dates, got: %v", result)
		}
	})

	t.Run("sorted_chronologically", func(t *testing.T) {
		t2 := time.Date(2025, 3, 15, 10, 0, 0, 0, time.UTC)
		t1 := time.Date(2025, 3, 14, 12, 0, 0, 0, time.UTC)
		events := []*gl.ContributionEvent{
			{CreatedAt: &t2},
			{CreatedAt: &t1},
			{CreatedAt: &t1},
		}
		result := groupEventsByDay(events)
		if len(result) != 2 {
			t.Fatalf("expected 2 days, got %d", len(result))
		}
		if result[0].date != "2025-03-14" || result[0].count != 2 {
			t.Errorf("first day = %+v, want 2025-03-14 count=2", result[0])
		}
		if result[1].date != "2025-03-15" || result[1].count != 1 {
			t.Errorf("second day = %+v, want 2025-03-15 count=1", result[1])
		}
	})
}

// TestProjectPathFromWebURL covers the web URL parsing helper edge cases.
func TestProjectPathFromWebURL(t *testing.T) {
	t.Run("valid_url", func(t *testing.T) {
		got := projectPathFromWebURL("https://gitlab.com/group/subgroup/project/-/issues/1")
		if got != "group/subgroup/project" {
			t.Errorf("got %q, want %q", got, "group/subgroup/project")
		}
	})

	t.Run("url_without_dash", func(t *testing.T) {
		got := projectPathFromWebURL("https://gitlab.com/group/project")
		if got != "" {
			t.Errorf("expected empty for URL without /-/, got %q", got)
		}
	})

	t.Run("malformed_url", func(t *testing.T) {
		got := projectPathFromWebURL("not-a-url")
		if got != "" {
			t.Errorf("expected empty for malformed URL, got %q", got)
		}
	})

	// dash_at_path_root exercises the trailing `return ""` branch where the
	// URL contains "/-/" but it appears immediately after the host (no group
	// segment to extract).
	t.Run("dash_at_path_root", func(t *testing.T) {
		got := projectPathFromWebURL("https://gitlab.com/-/snippets/1")
		if got != "" {
			t.Errorf("expected empty for /-/ at path root, got %q", got)
		}
	})
}

// TestStateArg_PerResourceVocabulary verifies each state filter offers exactly
// the states GitLab accepts for that resource.
//
// "merged" is a merge-request state: advertising it on an issue prompt only
// earns a 400 from the issues API, and the description is what a model reads.
func TestStateArg_PerResourceVocabulary(t *testing.T) {
	tests := []struct {
		name     string
		arg      *mcp.PromptArgument
		want     []string
		unwanted []string
	}{
		{
			name:     "issue states",
			arg:      issueStateArg("opened"),
			want:     []string{"opened", "closed", "all"},
			unwanted: []string{"merged"},
		},
		{
			name: "merge request states",
			arg:  mrStateArg("opened"),
			want: []string{"opened", "closed", "merged", "all"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.arg.Name != argState || tt.arg.Required {
				t.Fatalf("%s = %+v, want an optional %q argument", tt.name, tt.arg, argState)
			}
			for _, state := range tt.want {
				if !strings.Contains(tt.arg.Description, state) {
					t.Errorf("%s description = %q, want it to offer %q", tt.name, tt.arg.Description, state)
				}
			}
			for _, state := range tt.unwanted {
				if strings.Contains(tt.arg.Description, state) {
					t.Errorf("%s description = %q, want no %q state", tt.name, tt.arg.Description, state)
				}
			}
			if !strings.Contains(tt.arg.Description, "default: opened") {
				t.Errorf("%s description = %q, want the default named", tt.name, tt.arg.Description)
			}
		})
	}
}

// TestMermaidQuoted_ContainsAValueInsideTheDiagram verifies that a
// GitLab-authored value rendered into a Mermaid chart cannot leave the string
// it sits in.
//
// Mermaid has no backslash escape, so the %q these charts used to be built with
// wrote a backslash Mermaid renders literally and left the quotation mark that
// ends the label. A label or a contributor name is whatever somebody typed, so
// the quote is reachable: past it, the rest of the name is diagram syntax and
// can add entries or a title to a chart the server wrote.
func TestMermaidQuoted_ContainsAValueInsideTheDiagram(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain label", in: "bug", want: `"bug"`},
		{name: "empty label", in: "", want: `""`},
		{name: "spaces survive", in: "needs triage", want: `"needs triage"`},
		{name: "quote becomes the entity form", in: `a"b`, want: `"a#quot;b"`},
		{
			name: "a forged entry cannot start one",
			in:   "x\" : 1\n    \"y",
			want: `"x#quot; : 1     #quot;y"`,
		},
		{name: "hash is encoded first", in: "#quot;", want: `"#35;quot;"`},
		{name: "newline becomes a space", in: "a\nb", want: `"a b"`},
		{name: "carriage return becomes a space", in: "a\rb", want: `"a b"`},
		{name: "tab becomes a space", in: "a\tb", want: `"a b"`},
		{name: "control byte is dropped", in: "a\x00b", want: `"ab"`},
		{name: "non-ASCII is kept as itself", in: "café", want: "\"café\""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mermaidQuoted(tt.in); got != tt.want {
				t.Errorf("mermaidQuoted(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestTruncateRunes_CutsOnARuneBoundary verifies that shortening a
// GitLab-authored string never splits a character.
//
// A byte slice at a fixed offset leaves the orphaned bytes of whatever
// character spanned it in the message, and that is no longer UTF-8: the
// escapers pass them through, being neither control bytes nor Markdown, and the
// client renders a replacement glyph. Prose people type is exactly where a
// multi-byte character at the cut is likely.
func TestTruncateRunes_CutsOnARuneBoundary(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		limit int
		want  string
	}{
		{name: "shorter than the limit is untouched", in: "abc", limit: 10, want: "abc"},
		{name: "exactly the limit is untouched", in: "abc", limit: 3, want: "abc"},
		{name: "ascii cuts at the limit", in: "abcdef", limit: 3, want: "abc"},
		{name: "zero limit is empty", in: "abc", limit: 0, want: ""},
		{name: "negative limit is empty", in: "abc", limit: -1, want: ""},
		// U+00E9 is two bytes: a limit of 2 lands mid-character and must give
		// back the character before it rather than half of this one.
		{name: "two-byte rune is never split", in: "aéb", limit: 2, want: "a"},
		{name: "two-byte rune is kept when it fits", in: "aéb", limit: 3, want: "aé"},
		// U+1F600 is four bytes.
		{name: "four-byte rune is never split", in: "a\U0001F600b", limit: 4, want: "a"},
		{name: "four-byte rune is kept when it fits", in: "a\U0001F600b", limit: 5, want: "a\U0001F600"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateRunes(tt.in, tt.limit)
			if got != tt.want {
				t.Errorf("truncateRunes(%q, %d) = %q, want %q", tt.in, tt.limit, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("truncateRunes(%q, %d) = %q, which is not valid UTF-8", tt.in, tt.limit, got)
			}
		})
	}
}

// TestWriteClosingRule_SeparatesTheRuleFromWhateverCameBefore verifies that a
// prompt's closing thematic break always follows a blank line.
//
// A "---" written directly under a line of text is a setext heading in
// CommonMark: no rule is drawn, and the sentence above it becomes an H2. The
// reports that closed that way were the ones with nothing to report, whose last
// section is a sentence rather than a table, so the message a reader most needs
// to end cleanly was the one that did not.
func TestWriteClosingRule_SeparatesTheRuleFromWhateverCameBefore(t *testing.T) {
	tests := []struct {
		name        string
		before      string
		instruction string
		want        string
	}{
		{
			name:        "empty builder",
			before:      "",
			instruction: "Please analyze.",
			want:        "---\nPlease analyze.\n",
		},
		{
			name:        "after a sentence the rule gains its blank line",
			before:      "No stale items found.\n",
			instruction: "Please analyze.",
			want:        "No stale items found.\n\n---\nPlease analyze.\n",
		},
		{
			name:        "after a blank line nothing is added",
			before:      "| a | 1 |\n\n",
			instruction: "Please analyze.",
			want:        "| a | 1 |\n\n---\nPlease analyze.\n",
		},
		{
			name:        "several blank lines are left alone",
			before:      "x\n\n\n",
			instruction: "Please analyze.",
			want:        "x\n\n\n---\nPlease analyze.\n",
		},
		{
			name:        "mid-line text gets a line ending and a blank line",
			before:      "trailing text",
			instruction: "Please analyze.",
			want:        "trailing text\n\n---\nPlease analyze.\n",
		},
		{
			name:        "an instruction that ends in a newline gains no second one",
			before:      "x\n\n",
			instruction: "Please analyze.\n",
			want:        "x\n\n---\nPlease analyze.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			b.WriteString(tt.before)
			writeClosingRule(&b, tt.instruction)
			if got := b.String(); got != tt.want {
				t.Errorf("writeClosingRule() wrote %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDeduplicateMRs_TellsTwoProjectsApart verifies that merge requests are
// identified by project and IID together.
//
// An IID is unique inside a project and nowhere else, so !1 of one project and
// !1 of another are two merge requests. Both cross-project counters read this
// identity now; the authored set used to key on the IID alone.
func TestDeduplicateMRs_TellsTwoProjectsApart(t *testing.T) {
	tests := []struct {
		name      string
		a         []*gl.BasicMergeRequest
		b         []*gl.BasicMergeRequest
		wantCount int
	}{
		{name: "both empty", a: nil, b: nil, wantCount: 0},
		{
			name:      "the same MR twice is one",
			a:         []*gl.BasicMergeRequest{{ProjectID: 1, IID: 1}},
			b:         []*gl.BasicMergeRequest{{ProjectID: 1, IID: 1}},
			wantCount: 1,
		},
		{
			name:      "the same IID in two projects is two",
			a:         []*gl.BasicMergeRequest{{ProjectID: 1, IID: 1}},
			b:         []*gl.BasicMergeRequest{{ProjectID: 2, IID: 1}},
			wantCount: 2,
		},
		{
			name:      "two IIDs in one project are two",
			a:         []*gl.BasicMergeRequest{{ProjectID: 1, IID: 1}, {ProjectID: 1, IID: 2}},
			b:         nil,
			wantCount: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := len(deduplicateMRs(tt.a, tt.b)); got != tt.wantCount {
				t.Errorf("deduplicateMRs() returned %d merge request(s), want %d", got, tt.wantCount)
			}
		})
	}
}

// serverlessRegistrar keeps the handler [addPrompt] wrapped, so a test can call
// it with no server and no session behind it.
type serverlessRegistrar struct {
	handler mcp.PromptHandler
}

func (r *serverlessRegistrar) AddPrompt(_ *mcp.Prompt, handler mcp.PromptHandler) {
	r.handler = handler
}

// TestAddPrompt_TheDescriptionOnTheResult_IsFilledOnlyWhenTheHandlerLeftItEmpty
// verifies both halves of the description default every prompt goes through.
//
// The catalog carries a description for all 37 and prompts/get returned none,
// so a client that fetched a prompt without listing first had nothing to label
// it with; that is the filling half. The withholding half matters just as
// much and is the one nothing held: the condition is an `&&`, and read as an
// `||` it overwrites whatever a handler chose to say with the registration's
// own sentence. Neither direction is visible from the served surface today,
// because no handler sets a description — which is exactly why the rule has to
// be asserted here rather than through a prompt.
func TestAddPrompt_TheDescriptionOnTheResult_IsFilledOnlyWhenTheHandlerLeftItEmpty(t *testing.T) {
	tests := []struct {
		name          string
		fromHandler   string
		wantOnTheWire string
	}{
		{name: "an empty description takes the registration's", fromHandler: "", wantOnTheWire: "the registered one"},
		{name: "a description the handler chose is kept", fromHandler: "the handler's own", wantOnTheWire: "the handler's own"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &serverlessRegistrar{}
			addPrompt(reg,
				&mcp.Prompt{Name: "probe", Description: "the registered one"},
				func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
					return &mcp.GetPromptResult{Description: tt.fromHandler}, nil
				})
			if reg.handler == nil {
				t.Fatal("addPrompt registered no handler")
			}

			got, err := reg.handler(t.Context(), &mcp.GetPromptRequest{Params: &mcp.GetPromptParams{}})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Description != tt.wantOnTheWire {
				t.Errorf("description = %q, want %q", got.Description, tt.wantOnTheWire)
			}
		})
	}

	// A handler that answers with neither a result nor an error is the other
	// side of the nil check, and the one where filling the description in
	// would dereference nothing at all.
	t.Run("a handler that returns no result at all is passed through", func(t *testing.T) {
		reg := &serverlessRegistrar{}
		addPrompt(reg,
			&mcp.Prompt{Name: "probe", Description: "the registered one"},
			func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				return nil, nil //nolint:nilnil // the handler shape the nil check exists for
			})
		if reg.handler == nil {
			t.Fatal("addPrompt registered no handler")
		}

		got, err := reg.handler(t.Context(), &mcp.GetPromptRequest{Params: &mcp.GetPromptParams{}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("result = %+v, want the handler's own nil", got)
		}
	})
}

// TestExtractProjectPath_AReferenceCarryingOnlyTheMarker_FallsBackToTheProjectID
// verifies that a reference whose separator sits at position zero is treated as
// carrying no project path at all.
//
// Both extractors cut the reference at the last "!" or "#" and keep what is in
// front of it, and both require that index to be greater than zero. At zero
// there is nothing in front: keeping `ref[:0]` would put an empty project
// heading over the rows, and the fallback to "project-N" is the answer that
// still tells a reader which project the row came from.
func TestExtractProjectPath_AReferenceCarryingOnlyTheMarker_FallsBackToTheProjectID(t *testing.T) {
	t.Run("merge request", func(t *testing.T) {
		mr := &gl.BasicMergeRequest{
			ProjectID:  7,
			References: &gl.IssueReferences{Full: "!42"},
		}
		if got := extractProjectPath(mr); got != "project-7" {
			t.Errorf(fmtExtractProjectPath, got, "project-7")
		}
	})

	t.Run("issue", func(t *testing.T) {
		issue := &gl.Issue{
			ProjectID:  9,
			References: &gl.IssueReferences{Full: "#42"},
		}
		if got := extractIssueProjectPath(issue); got != "project-9" {
			t.Errorf("extractIssueProjectPath() = %q, want %q", got, "project-9")
		}
	})

	// GitLab sends a references object whose fields are empty on an item it
	// could not name, and the web URL is then the only thing left to read the
	// project out of.
	t.Run("an empty reference falls through to the web URL", func(t *testing.T) {
		mr := &gl.BasicMergeRequest{
			ProjectID:  7,
			References: &gl.IssueReferences{},
			WebURL:     "https://gitlab.example.com/group/project/-/merge_requests/42",
		}
		if got := extractProjectPath(mr); got != "group/project" {
			t.Errorf(fmtExtractProjectPath, got, "group/project")
		}

		issue := &gl.Issue{
			ProjectID:  9,
			References: &gl.IssueReferences{},
			WebURL:     "https://gitlab.example.com/group/other/-/issues/42",
		}
		if got := extractIssueProjectPath(issue); got != "group/other" {
			t.Errorf("extractIssueProjectPath() = %q, want %q", got, "group/other")
		}
	})
}

// TestProjectPathFromWebURL_AWebURLWithNothingBeforeASeparator_YieldsNoPath
// verifies that each of the three offsets this parser takes is required to be
// greater than zero, by feeding it the values where they are exactly zero.
//
// A web URL is GitLab-authored data reaching a cross-project prompt's project
// heading, so what the parser does with a value that is not the shape it
// expects is part of its contract rather than a curiosity: a scheme with no
// host and a path with no host both have a separator at offset zero, and
// accepting either would name the project after whatever followed.
func TestProjectPathFromWebURL_AWebURLWithNothingBeforeASeparator_YieldsNoPath(t *testing.T) {
	tests := []struct {
		name   string
		webURL string
		want   string
	}{
		{
			name:   "a scheme separator at the start is not a scheme to strip",
			webURL: "://gitlab.example.com/group/project/-/merge_requests/1",
			want:   "/gitlab.example.com/group/project",
		},
		{
			name:   "a leading slash is not a host to strip",
			webURL: "/group/project/-/merge_requests/1",
			want:   "/group/project",
		},
		{
			name:   "the ordinary shape still resolves",
			webURL: "https://gitlab.example.com/group/project/-/merge_requests/1",
			want:   "group/project",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := projectPathFromWebURL(tt.webURL); got != tt.want {
				t.Errorf("projectPathFromWebURL(%q) = %q, want %q", tt.webURL, got, tt.want)
			}
		})
	}
}

// TestTruncateRunes_AValueThatIsNotUTF8_IsStillCutRatherThanPanicking verifies
// that the walk back to a rune boundary stops at the start of the string.
//
// The loop steps backwards while the byte it is looking at is a continuation
// byte, and its `cut > 0` is what stops it at offset zero. A string whose very
// first byte is a continuation byte is the only value that reaches that stop,
// and without it the index goes to -1 and the slice expression panics — taking
// down whichever prompt was shortening a description at the time. GitLab sends
// UTF-8, so nothing here should ever see one; a truncation helper that panics
// on bytes it was handed is still a defect, and nothing else can catch it.
func TestTruncateRunes_AValueThatIsNotUTF8_IsStillCutRatherThanPanicking(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		limit int
		want  string
	}{
		{name: "every byte is a continuation byte", in: "\x80\x80\x80", limit: 1, want: ""},
		{name: "the walk back reaches the start of the string", in: "\xa9\xa9\xa9\xa9", limit: 2, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateRunes(tt.in, tt.limit); got != tt.want {
				t.Errorf("truncateRunes(%q, %d) = %q, want %q", tt.in, tt.limit, got, tt.want)
			}
		})
	}
}

// TestWriteIssueTable_AnIssueWithNoLabels_RendersAHyphen verifies that the
// labels cell of an issue carrying none says so.
//
// The cell is filled from a join over the label names, and the join over an
// empty list is the empty string: a cell reading "|  |" is a cell a model has
// to guess about, where "-" is the same word every other absent value in these
// tables uses. The guard that chooses between them is a `len(...) > 0` nothing
// held to its boundary.
func TestWriteIssueTable_AnIssueWithNoLabels_RendersAHyphen(t *testing.T) {
	created := time.Now().Add(-48 * time.Hour)
	var b strings.Builder
	writeIssueTable(&b, []*gl.Issue{
		{IID: 1, Title: "Unlabeled", CreatedAt: &created},
		{IID: 2, Title: "Labeled", Labels: gl.Labels{"bug"}, CreatedAt: &created},
	})

	got := b.String()
	if !strings.Contains(got, "| #1 | Unlabeled | - |") {
		t.Errorf("an issue with no labels should render a hyphen, got:\n%s", got)
	}
	if !strings.Contains(got, "| #2 | Labeled | bug |") {
		t.Errorf("an issue with labels should render them, got:\n%s", got)
	}
}

// TestFormatAge_AtEachUnitBoundary_TakesTheLargerUnit pins the four thresholds
// this formatter switches on to the exact day they change.
//
// Every one of them is a `<` whose neighbors were only ever tested from well
// inside their own range, so each could be read as `<=` — one day, one week,
// one month and one year all reported as the unit below — without a single
// assertion moving. An age is what a reader uses to decide something is stale,
// so an off-by-one day at a threshold is the part of it worth pinning.
func TestFormatAge_AtEachUnitBoundary_TakesTheLargerUnit(t *testing.T) {
	day := 24 * time.Hour
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{name: "just under a day", duration: day - time.Minute, want: "<1d"},
		{name: "exactly one day", duration: day, want: "1d"},
		{name: "six days", duration: 6 * day, want: "6d"},
		{name: "exactly one week", duration: 7 * day, want: "1w"},
		{name: "twenty-nine days", duration: 29 * day, want: "4w"},
		{name: "exactly thirty days", duration: 30 * day, want: "1mo"},
		{name: "three hundred and sixty-four days", duration: 364 * day, want: "12mo"},
		{name: "exactly a year", duration: 365 * day, want: "1y"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatAge(tt.duration); got != tt.want {
				t.Errorf("formatAge(%v) = %q, want %q", tt.duration, got, tt.want)
			}
		})
	}
}

// TestMergeDuration_OnlyOneOfTheTwoTimestamps_IsZeroRatherThanADereference
// verifies that the nil check covers each timestamp on its own.
//
// It is an `||` over two pointers, and read as an `&&` a merge request with a
// creation date and no merge date passes the guard and dereferences the nil
// one. GitLab sends exactly that for every open merge request, and
// merge_velocity hands it every merge request it lists.
func TestMergeDuration_OnlyOneOfTheTwoTimestamps_IsZeroRatherThanADereference(t *testing.T) {
	created := time.Now().Add(-48 * time.Hour)
	merged := time.Now()
	tests := []struct {
		name string
		mr   *gl.BasicMergeRequest
		want time.Duration
	}{
		{name: "created but never merged", mr: &gl.BasicMergeRequest{CreatedAt: &created}, want: 0},
		{name: "merged with no creation date", mr: &gl.BasicMergeRequest{MergedAt: &merged}, want: 0},
		{name: "neither", mr: &gl.BasicMergeRequest{}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mergeDuration(tt.mr); got != tt.want {
				t.Errorf("mergeDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}
