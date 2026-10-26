package toolutil

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// mergeRequestDiffPositionMaxPages bounds the diff pages one advisory position
// check reads (register row RQB-013). Reaching it defers to GitLab's write.
const mergeRequestDiffPositionMaxPages = 100

// MergeRequestDiffForPositionCheck finds targetPath in a merge request's current
// diff and reports whether the result belongs to the position's expected refs.
// A nil diff with a true second result means the file was absent from a complete
// listing. A false second result means the caller must defer validation to
// GitLab, including when the snapshot cannot be confirmed or a request fails.
//
// The search stops at the matching old or new path and retains no earlier
// pages. It reads at most 100 pages of 100 files. The MR's diff refs must match
// expected before and after the search; this detects changes between reads,
// not changes after the final read. GitLab still judges the subsequent write.
func MergeRequestDiffForPositionCheck(ctx context.Context, client *gitlabclient.Client, projectID string, mrIID int64, targetPath string, expected gl.MergeRequestDiffRefs) (*gl.MergeRequestDiff, bool) {
	if expected.BaseSha == "" || expected.StartSha == "" || expected.HeadSha == "" {
		return nil, false
	}
	if !mergeRequestPositionSnapshotMatches(ctx, client, projectID, mrIID, expected) {
		return nil, false
	}
	opts := &gl.ListMergeRequestDiffsOptions{Page: 1, PerPage: 100}
	for range mergeRequestDiffPositionMaxPages {
		page, resp, err := client.GL().MergeRequests.ListMergeRequestDiffs(projectID, mrIID, opts, gl.WithContext(ctx))
		if err != nil {
			slog.DebugContext(ctx, "merge request diff listing failed; GitLab judges the position",
				"project_id", projectID, "merge_request_iid", mrIID, "page", opts.Page, "error", err)
			return nil, false
		}
		for _, diff := range page {
			if diff != nil && (diff.NewPath == targetPath || diff.OldPath == targetPath) {
				if !mergeRequestPositionSnapshotMatches(ctx, client, projectID, mrIID, expected) {
					return nil, false
				}
				return diff, true
			}
		}
		if resp == nil || resp.NextPage == 0 {
			return nil, mergeRequestPositionSnapshotMatches(ctx, client, projectID, mrIID, expected)
		}
		if resp.NextPage <= opts.Page {
			slog.DebugContext(ctx, "merge request diff pagination did not advance; GitLab judges the position",
				"project_id", projectID, "merge_request_iid", mrIID, "page", opts.Page, "next_page", resp.NextPage)
			return nil, false
		}
		opts.Page = resp.NextPage
	}
	slog.DebugContext(ctx, "merge request diff page budget exhausted; GitLab judges the position",
		"project_id", projectID, "merge_request_iid", mrIID)
	return nil, false
}

// mergeRequestPositionSnapshotMatches confirms that the current diff has all
// three expected refs. An unreadable snapshot cannot justify a local refusal.
func mergeRequestPositionSnapshotMatches(ctx context.Context, client *gitlabclient.Client, projectID string, mrIID int64, expected gl.MergeRequestDiffRefs) bool {
	if ctx.Err() != nil {
		return false
	}
	mr, _, err := client.GL().MergeRequests.GetMergeRequest(projectID, mrIID, nil, gl.WithContext(ctx))
	if err != nil {
		slog.DebugContext(ctx, "merge request diff snapshot unreadable; GitLab judges the position",
			"project_id", projectID, "merge_request_iid", mrIID, "error", err)
		return false
	}
	if mr == nil || mr.DiffRefs != expected {
		slog.DebugContext(ctx, "merge request diff snapshot differs; GitLab judges the position",
			"project_id", projectID, "merge_request_iid", mrIID)
		return false
	}
	return true
}

// LineType classifies a line within a unified diff hunk.
type LineType int

const (
	// LineContext identifies an unchanged line present in both old and new file versions.
	LineContext LineType = iota // Unchanged line (present in both old and new file)
	// LineAdded identifies a line present only in the new file version.
	LineAdded // Added line (present only in new file)
	// LineRemoved identifies a line present only in the old file version.
	LineRemoved // Removed line (present only in old file)
)

// DiffLine represents a single line in a parsed unified diff with its
// old/new line numbers and type (added, removed, or context).
type DiffLine struct {
	OldLine int      // Line number in old file (0 for added lines)
	NewLine int      // Line number in new file (0 for removed lines)
	Type    LineType // Whether the line was added, removed, or unchanged
}

// ParseDiffLines parses a unified diff string and returns metadata for each
// line, including old/new line numbers and whether it is added, removed, or
// context. Only lines inside @@ hunk headers are returned.
func ParseDiffLines(diff string) []DiffLine {
	if diff == "" {
		return nil
	}
	var lines []DiffLine
	var oldLine, newLine int

	for line := range strings.SplitSeq(diff, "\n") {
		if strings.HasPrefix(line, "@@") {
			oldLine, newLine = parseHunkHeader(line)
			continue
		}
		if oldLine == 0 && newLine == 0 {
			continue
		}

		switch {
		case strings.HasPrefix(line, "+"):
			lines = append(lines, DiffLine{NewLine: newLine, Type: LineAdded})
			newLine++
		case strings.HasPrefix(line, "-"):
			lines = append(lines, DiffLine{OldLine: oldLine, Type: LineRemoved})
			oldLine++
		case strings.HasPrefix(line, " "):
			lines = append(lines, DiffLine{OldLine: oldLine, NewLine: newLine, Type: LineContext})
			oldLine++
			newLine++
		}
		// Skip empty strings (trailing newline artifact) and metadata
		// lines like "\ No newline at end of file"
	}
	return lines
}

// parseHunkHeader extracts the starting line numbers from a unified diff hunk
// header. Format: @@ -oldStart[,oldCount] +newStart[,newCount] @@.
//
// A header it cannot read is refused whole, as (0, 0), the same answer a line
// that is not a hunk header gets: [ParseDiffLines] then skips the hunk's lines
// until the next header. Reading a start it could not parse as 0 instead
// would number every line of the hunk wrongly rather than not at all, and a
// position validated against those numbers would be wrong by the real start.
func parseHunkHeader(line string) (oldStart, newStart int) {
	parts := strings.SplitN(line, "@@", 3)
	if len(parts) < 3 {
		return 0, 0
	}
	for r := range strings.FieldsSeq(strings.TrimSpace(parts[1])) {
		var target *int
		switch {
		case strings.HasPrefix(r, "-"):
			target = &oldStart
		case strings.HasPrefix(r, "+"):
			target = &newStart
		default:
			continue
		}
		start, err := strconv.Atoi(strings.SplitN(r[1:], ",", 2)[0])
		if err != nil {
			return 0, 0
		}
		*target = start
	}
	return oldStart, newStart
}

// ValidateDiffPosition checks whether a (newLine, oldLine) combination
// corresponds to a valid commentable position in the parsed diff lines.
//
// Rules enforced (per GitLab API):
//   - new_line only → line must be an added (+) line
//   - old_line only → line must be a removed (-) line
//   - both set      → line must be an unchanged context line
//
// Returns nil when the position is valid, or a descriptive error explaining
// exactly why the position is invalid and what the caller should do instead.
func ValidateDiffPosition(diffLines []DiffLine, newLine, oldLine int) error {
	if len(diffLines) == 0 {
		return errors.New("no diff content available for this file")
	}
	if newLine == 0 && oldLine == 0 {
		return errors.New("at least one of new_line or old_line must be set")
	}

	for _, dl := range diffLines {
		if matched, err := validateDiffLinePosition(dl, newLine, oldLine); matched {
			return err
		}
	}

	return buildPositionError(diffLines, newLine, oldLine)
}

func validateDiffLinePosition(dl DiffLine, newLine, oldLine int) (bool, error) {
	// The first test takes every call with both lines set, so each test after
	// it already knows the other line is zero. Written as ifs rather than a
	// tagless switch: a case expression carries no statement counter, so the
	// mutation gate could not see these conditions.
	if newLine != 0 && oldLine != 0 {
		return dl.Type == LineContext && dl.NewLine == newLine && dl.OldLine == oldLine, nil
	}
	if newLine != 0 {
		return validateNewLineOnlyPosition(dl, newLine)
	}
	if oldLine != 0 {
		return validateOldLineOnlyPosition(dl, oldLine)
	}
	return false, nil
}

func validateNewLineOnlyPosition(dl DiffLine, newLine int) (bool, error) {
	if dl.Type == LineAdded && dl.NewLine == newLine {
		return true, nil
	}
	if dl.Type == LineContext && dl.NewLine == newLine {
		return true, fmt.Errorf(
			"new_line %d is an unchanged context line, not an added line. "+
				"for context lines set BOTH old_line=%d and new_line=%d. "+
				"Use new_line alone only for added (+) lines",
			newLine, dl.OldLine, dl.NewLine,
		)
	}
	return false, nil
}

func validateOldLineOnlyPosition(dl DiffLine, oldLine int) (bool, error) {
	if dl.Type == LineRemoved && dl.OldLine == oldLine {
		return true, nil
	}
	if dl.Type == LineContext && dl.OldLine == oldLine {
		return true, fmt.Errorf(
			"old_line %d is an unchanged context line, not a removed line. "+
				"for context lines set BOTH old_line=%d and new_line=%d. "+
				"Use old_line alone only for removed (-) lines",
			oldLine, dl.OldLine, dl.NewLine,
		)
	}
	return false, nil
}

// buildPositionError constructs a descriptive error when a line is not found
// in the diff at all, listing the valid line ranges.
func buildPositionError(diffLines []DiffLine, newLine, oldLine int) error {
	// Zero is "no such line" here, so a range starts at the first line that
	// has a number rather than at zero.
	var minNew, maxNew, minOld, maxOld int
	for _, dl := range diffLines {
		if dl.NewLine != 0 {
			if minNew == 0 {
				minNew = dl.NewLine
			}
			minNew = min(minNew, dl.NewLine)
			maxNew = max(maxNew, dl.NewLine)
		}
		if dl.OldLine != 0 {
			if minOld == 0 {
				minOld = dl.OldLine
			}
			minOld = min(minOld, dl.OldLine)
			maxOld = max(maxOld, dl.OldLine)
		}
	}

	target := ""
	if newLine != 0 {
		target = fmt.Sprintf("new_line %d", newLine)
	}
	if oldLine != 0 {
		if target != "" {
			target += " and "
		}
		target += fmt.Sprintf("old_line %d", oldLine)
	}

	return fmt.Errorf(
		"position (%s) is outside the diff range. Inline comments can only be placed on lines "+
			"visible in the diff context (valid new_line: %d-%d, valid old_line: %d-%d). "+
			"To comment on code outside the diff, omit the position parameter to create a general discussion instead. "+
			"See https://docs.gitlab.com/api/discussions/#create-a-new-thread-in-the-merge-request-diff",
		target, minNew, maxNew, minOld, maxOld,
	)
}
