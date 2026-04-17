// prompt_helpers_unit_test.go contains unit tests for prompt helper functions.
package prompts

import (
	"errors"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

const (
	fmtExtractProjectPath = "extractProjectPath() = %q, want %q"
	testGroupAlpha        = "group/alpha"
	testGroupBeta         = "group/beta"
	fmtMRAge              = "mrAge() = %q, want %q"
)

// TestParseDays_ValidInput validates parse days valid input across multiple scenarios using table-driven subtests.
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

// TestParseDays_InvalidInput validates parse days invalid input across multiple scenarios using table-driven subtests.
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

// TestSinceDate_ReturnsUTCPast verifies the behavior of since date returns u t c past.
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

// TestExtractProjectPath_FromReferences verifies the behavior of extract project path from references.
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

// TestExtractProjectPath_FromWebURL verifies the behavior of extract project path from web u r l.
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

// TestExtractProjectPath_FallbackToProjectID verifies the behavior of extract project path fallback to project i d.
func TestExtractProjectPath_FallbackToProjectID(t *testing.T) {
	mr := &gl.BasicMergeRequest{ProjectID: 789}
	got := extractProjectPath(mr)
	if got != "project-789" {
		t.Errorf(fmtExtractProjectPath, got, "project-789")
	}
}

// TestExtractIssueProjectPath_FromReferences verifies the behavior of extract issue project path from references.
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

// TestGroupMRsByProject_MultipleProjects verifies the behavior of group m rs by project multiple projects.
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

// TestGroupMRsByProject_EmptyInput verifies the behavior of group m rs by project empty input.
func TestGroupMRsByProject_EmptyInput(t *testing.T) {
	grouped := groupMRsByProject(nil)
	if len(grouped) != 0 {
		t.Errorf("expected empty map, got %d entries", len(grouped))
	}
}

// TestGroupIssuesByProject_MultipleProjects verifies the behavior of group issues by project multiple projects.
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

// TestMRAge_Days verifies the behavior of m r age days.
func TestMRAge_Days(t *testing.T) {
	created := time.Now().Add(-3 * 24 * time.Hour)
	mr := &gl.BasicMergeRequest{CreatedAt: &created}
	got := mrAge(mr)
	if got != "3d" {
		t.Errorf(fmtMRAge, got, "3d")
	}
}

// TestMRAge_Weeks verifies the behavior of m r age weeks.
func TestMRAge_Weeks(t *testing.T) {
	created := time.Now().Add(-14 * 24 * time.Hour)
	mr := &gl.BasicMergeRequest{CreatedAt: &created}
	got := mrAge(mr)
	if got != "2w" {
		t.Errorf(fmtMRAge, got, "2w")
	}
}

// TestMRAge_NilCreatedAt verifies the behavior of m r age nil created at.
func TestMRAge_NilCreatedAt(t *testing.T) {
	mr := &gl.BasicMergeRequest{}
	got := mrAge(mr)
	if got != "?" {
		t.Errorf(fmtMRAge, got, "?")
	}
}

// TestIssueAge_Days verifies the behavior of issue age days.
func TestIssueAge_Days(t *testing.T) {
	created := time.Now().Add(-5 * 24 * time.Hour)
	issue := &gl.Issue{CreatedAt: &created}
	got := issueAge(issue)
	if got != "5d" {
		t.Errorf("issueAge() = %q, want %q", got, "5d")
	}
}

// TestFormatAge_AllRanges validates format age all ranges across multiple scenarios using table-driven subtests.
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

// TestPipelineEmoji_AllStatuses validates pipeline emoji all statuses across multiple scenarios using table-driven subtests.
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

// TestWriteMRTable_Format verifies the behavior of write m r table format.
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
	if !strings.Contains(output, "feature/fix → develop") {
		t.Error("expected branch info")
	}
}

// TestWriteMRTable_EmptyList verifies the behavior of write m r table empty list.
func TestWriteMRTable_EmptyList(t *testing.T) {
	var b strings.Builder
	writeMRTable(&b, nil)
	if !strings.Contains(b.String(), "No merge requests found") {
		t.Error("expected empty message")
	}
}

// TestWriteIssueTable_Format verifies the behavior of write issue table format.
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

// TestWriteIssueTable_EmptyList verifies the behavior of write issue table empty list.
func TestWriteIssueTable_EmptyList(t *testing.T) {
	var b strings.Builder
	writeIssueTable(&b, nil)
	if !strings.Contains(b.String(), "No issues found") {
		t.Error("expected empty message")
	}
}

// TestMRStatusDraft_WithConflicts verifies the behavior of m r status draft with conflicts.
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

// TestMRStatus_NoFlags verifies the behavior of m r status no flags.
func TestMRStatus_NoFlags(t *testing.T) {
	mr := &gl.BasicMergeRequest{}
	got := mrStatus(mr)
	if got != "—" {
		t.Errorf("expected dash, got %q", got)
	}
}

// TestMergeDuration verifies the behavior of merge duration.
func TestMergeDuration(t *testing.T) {
	created := time.Now().Add(-48 * time.Hour)
	merged := time.Now()
	mr := &gl.BasicMergeRequest{CreatedAt: &created, MergedAt: &merged}
	d := mergeDuration(mr)
	if d < 47*time.Hour || d > 49*time.Hour {
		t.Errorf("mergeDuration() = %v, expected ~48h", d)
	}
}

// TestMergeDuration_NilTimestamps verifies the behavior of merge duration nil timestamps.
func TestMergeDuration_NilTimestamps(t *testing.T) {
	mr := &gl.BasicMergeRequest{}
	if d := mergeDuration(mr); d != 0 {
		t.Errorf("mergeDuration() = %v, want 0", d)
	}
}

// TestFormatDuration validates format duration across multiple scenarios using table-driven subtests.
func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		d        time.Duration
		expected string
	}{
		{"zero", 0, "—"},
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

// TestAvgDuration verifies the behavior of avg duration.
func TestAvgDuration(t *testing.T) {
	durations := []time.Duration{2 * time.Hour, 4 * time.Hour, 6 * time.Hour}
	avg := avgDuration(durations)
	if avg != 4*time.Hour {
		t.Errorf("avgDuration() = %v, want 4h", avg)
	}
}

// TestAvgDuration_Empty verifies the behavior of avg duration empty.
func TestAvgDuration_Empty(t *testing.T) {
	if avg := avgDuration(nil); avg != 0 {
		t.Errorf("avgDuration(nil) = %v, want 0", avg)
	}
}

// TestMedianDuration validates median duration across multiple scenarios using table-driven subtests.
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

// TestProgressBar validates progress bar across multiple scenarios using table-driven subtests.
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

// TestDeduplicateMRs verifies the behavior of deduplicate m rs.
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

// TestGetArgOr verifies the behavior of get arg or.
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

// TestSortedKeys verifies the behavior of sorted keys.
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

// TestReadinessLabel covers all three branches of readinessLabel.
func TestReadinessLabel(t *testing.T) {
	tests := []struct {
		blockers int
		wantSub  string
	}{
		{10, "Not Ready"},
		{3, "Needs Attention"},
		{0, "Ready"},
	}
	for _, tt := range tests {
		got := readinessLabel(tt.blockers)
		if !strings.Contains(got, tt.wantSub) {
			t.Errorf("readinessLabel(%d) = %q, want containing %q", tt.blockers, got, tt.wantSub)
		}
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
	if got := accessLevelIcon(gl.EnabledAccessControl); got == "" {
		t.Error("expected non-empty icon for enabled")
	}
	if got := accessLevelIcon(gl.DisabledAccessControl); got == "" {
		t.Error("expected non-empty icon for disabled")
	}
	if got := accessLevelIcon(""); got == "" {
		t.Error("expected non-empty icon for empty")
	}
}

// TestFormatTimePtr covers both branches.
func TestFormatTimePtr(t *testing.T) {
	if got := formatTimePtr(nil); got != "—" {
		t.Errorf("formatTimePtr(nil) = %q, want %q", got, "—")
	}
	now := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)
	if got := formatTimePtr(&now); got != "2025-03-15" {
		t.Errorf("formatTimePtr() = %q, want %q", got, "2025-03-15")
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
		writeFullAccessSection(&b, members, nil)
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
		writeFullAccessSection(&b, nil, groups)
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
		writeFullAccessSection(&b, nil, nil)
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
		writeFullLabelsSection(&b, labels)
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
		writeFullLabelsSection(&b, labels)
		out := b.String()
		if strings.Contains(out, "without description") {
			t.Errorf("should not warn when all have descriptions, got: %s", out)
		}
	})

	t.Run("no_labels", func(t *testing.T) {
		var b strings.Builder
		writeFullLabelsSection(&b, nil)
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
		})
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
		})
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
		writeFullPushRulesSection(&b, nil)
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
		})
		out := b.String()
		if !strings.Contains(out, "Prevent secrets") {
			t.Errorf("expected push rule rows, got: %s", out)
		}
		if !strings.Contains(out, "^(feat|fix):") {
			t.Errorf("expected commit regex, got: %s", out)
		}
	})
}

// TestFormatAccessLevels verifies access level formatting for protected branches.
func TestFormatAccessLevels(t *testing.T) {
	t.Run("empty_levels", func(t *testing.T) {
		if got := formatAccessLevels(nil); got != "—" {
			t.Errorf("formatAccessLevels(nil) = %q, want %q", got, "—")
		}
	})

	t.Run("multiple_levels", func(t *testing.T) {
		levels := []*gl.BranchAccessDescription{
			{AccessLevel: 40},
			{AccessLevel: 30},
		}
		got := formatAccessLevels(levels)
		if !strings.Contains(got, "Maintainer") {
			t.Errorf("expected Maintainer, got: %s", got)
		}
		if !strings.Contains(got, "Developer") {
			t.Errorf("expected Developer, got: %s", got)
		}
	})
}

// TestWriteFullBranchSection verifies branch protection section in full audit.
func TestWriteFullBranchSection(t *testing.T) {
	t.Run("no_branches", func(t *testing.T) {
		var b strings.Builder
		writeFullBranchSection(&b, nil)
		out := b.String()
		if !strings.Contains(out, "**Protected branches:** 0") {
			t.Errorf("expected zero count, got: %s", out)
		}
	})

	t.Run("with_branches", func(t *testing.T) {
		var b strings.Builder
		branches := []*gl.ProtectedBranch{
			{
				Name:            "main",
				AllowForcePush:  false,
				PushAccessLevels: []*gl.BranchAccessDescription{{AccessLevel: 40}},
			},
		}
		writeFullBranchSection(&b, branches)
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
			Name:                   "feature",
			UnprotectAccessLevels:  []*gl.BranchAccessDescription{{AccessLevel: 40}},
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
}