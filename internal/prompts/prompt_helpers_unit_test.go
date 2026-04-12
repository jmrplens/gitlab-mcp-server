// prompt_helpers_unit_test.go contains unit tests for prompt helper functions.
package prompts

import (
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
