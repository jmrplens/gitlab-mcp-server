// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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
//
// The check exists to hand a model an actionable message before GitLab
// answers a line outside the diff with a bare 400 or 500, and it is advisory:
// whenever the second result is false the caller skips the check rather than
// refusing the write. What is skipped is the message, never the judgement,
// because the write that follows goes to the same instance with the same
// credential and GitLab answers it on its own terms. A 401, 403 or 404 on the
// listing or on the snapshot read is the refusal the write is about to get,
// with the right status; a cancelled context ends the write the same way; and
// a transient failure, a snapshot that moved or an exhausted page budget
// leaves the write judged by GitLab alone, which is exactly what a caller had
// before the check existed. The cause is logged at debug so that a check
// skipped every time can be seen without the write having to fail.
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
		if resp.NextPage == 0 {
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

// The position types GitLab's merge request and commit discussion routes and
// its draft note routes accept: each declares position_type with values text,
// image and file (lib/api/discussions.rb and lib/api/draft_notes.rb), and the
// tools send text when the caller names none.
const (
	// PositionTypeText places a comment on a line of a text diff.
	PositionTypeText = "text"
	// PositionTypeImage places a comment on a point of an image.
	PositionTypeImage = "image"
	// PositionTypeFile places a comment on a whole file.
	PositionTypeFile = "file"
)

// ValidatePositionAnchor refuses a position, whatever its type, that lacks a
// part of the anchor GitLab completes every position by: the start and head
// commits of the diff, and the file, named by its new or its old path.
// Gitlab::Diff::Position#complete? asks for a file path (new_path, or
// old_path when new_path is blank) and for DiffRefs#complete?, which is
// start_sha and head_sha, before it asks the position type's formatter for
// the rest, and a value of blanks is not present there. The base commit is
// not part of it, so it is not judged here.
//
// It runs before the check of the position type and before any request, for
// the reason [ValidateImagePosition] gives: GitLab's answer names no field,
// and for a draft note it is no answer at all. Grape requires base_sha,
// start_sha and head_sha only to be present as keys, so an empty value
// passes it.
func ValidatePositionAnchor(startSHA, headSHA, oldPath, newPath string) error {
	var missing []string
	if strings.TrimSpace(startSHA) == "" {
		missing = append(missing, "start_sha")
	}
	if strings.TrimSpace(headSHA) == "" {
		missing = append(missing, "head_sha")
	}
	if strings.TrimSpace(newPath) == "" && strings.TrimSpace(oldPath) == "" {
		missing = append(missing, "a path in new_path or old_path")
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"a position of any type needs start_sha, head_sha and the file's new_path or old_path (missing: %s): "+
			"GitLab places every inline comment by the start and head commits of the diff and the file it is on, "+
			"and keeps no position without them",
		strings.Join(missing, ", "),
	)
}

// PositionNewPath returns the new_path a merge request position sends, nil
// when it is left out or blank so that the key is not sent at all. GitLab
// reads a blank new_path as absent and completes the position with old_path
// (Position#file_path is new_path.presence || old_path), but it looks the
// file's diff up by the paths the position carries,
// [old_path, new_path].compact.uniq (Position#diff_options), which keeps an
// empty string, and Gitaly passes each to git diff as a pathspec, which git
// refuses for an empty one ("empty string is not a valid pathspec"). Sent
// as "", a position naming only old_path failed there instead of being
// placed.
func PositionNewPath(newPath string) *string {
	if strings.TrimSpace(newPath) == "" {
		return nil
	}
	return &newPath
}

// ValidateTextPosition refuses a text position that names no line. GitLab
// completes a text position only when its old_line or its new_line is present
// (Gitlab::Diff::Formatters::TextFormatter#complete?), a line_range included,
// whose own endpoints do not stand in for them. Which line it may be is
// judged against the merge request's diff by [ValidateDiffPosition], when the
// diff can be read; this check needs no request, so it holds even when that
// one is skipped. A line is numbered from 1, so 0 is no line, as the tools
// send none for it, and a negative number is refused: GitLab declares both
// lines a bare Integer and takes one as present, so it saves a draft note
// placed on line -1 and answers a discussion with a 400 about a line_code,
// which the caller did not send.
func ValidateTextPosition(oldLine, newLine int64) error {
	if oldLine < 0 || newLine < 0 {
		return fmt.Errorf(
			"new_line and old_line are line numbers counted from 1 (got new_line %d, old_line %d): "+
				"leave out the one the line does not have rather than sending a negative number",
			newLine, oldLine,
		)
	}
	if oldLine != 0 || newLine != 0 {
		return nil
	}
	return errors.New(
		"a text position needs new_line or old_line: GitLab places a line comment by the line's number in the " +
			"new file (an added line), in the old file (a removed line) or in both (an unchanged line), " +
			"and a line_range does not stand in for it",
	)
}

// ImageCoordinate converts an image coordinate to the type client-go's
// gl.PositionOptions gives x and y. GitLab declares both as integers on the
// discussion and draft note routes (lib/api/discussions.rb and
// lib/api/draft_notes.rb), and Grape coerces a fraction to an integer
// without a word, so the tools take whole pixels and convert them here, where
// a float64 holds each of them exactly. A nil coordinate stays nil, so a
// field left out is not sent. The client-go side is recorded in
// docs/development/upstream-bugs.md, and this retires when x and y take an
// *int64 there.
func ImageCoordinate(v *int64) *float64 {
	if v == nil {
		return nil
	}
	return new(float64(*v))
}

// ValidateImagePosition refuses an image position that lacks one of the four
// fields GitLab places an image comment by. GitLab completes an image
// position only when its width, height, x and y are all present
// (Gitlab::Diff::Formatters::ImageFormatter#complete?), and a 0 is present
// there, so a field is missing here only when it was not given. Nothing else
// of an image position is judged: GitLab computes no line code for one (a
// DiffNote validates its line code only on a text position), so the lines of
// the merge request's diff have nothing to say about it.
//
// The check runs before any request because GitLab's own answers to an
// incomplete position name no field, and differ by route: a discussion is
// refused with a 400 saying the position is incomplete, and a draft note is
// saved and later published as a general comment with its position dropped
// (DraftNote#publish_params keeps the position only when it is complete).
func ValidateImagePosition(width, height, x, y *int64) error {
	var missing []string
	if width == nil {
		missing = append(missing, "width")
	}
	if height == nil {
		missing = append(missing, "height")
	}
	if x == nil {
		missing = append(missing, "x")
	}
	if y == nil {
		missing = append(missing, "y")
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"an image position needs width, height, x and y (missing: %s): GitLab places an image comment by "+
			"the image's width and height in pixels and the x and y of the point on it, and reads no line of it",
		strings.Join(missing, ", "),
	)
}

// ValidateFilePosition refuses a file position that lacks its old or its new
// path. GitLab completes a file position only when both are present
// (Gitlab::Diff::Formatters::FileFormatter#complete?), where a path of blanks
// is not, and reads no line of one, so the lines of the merge request's diff
// are not consulted. It runs before any request for the reason
// [ValidateImagePosition] gives.
func ValidateFilePosition(oldPath, newPath string) error {
	var missing []string
	if strings.TrimSpace(oldPath) == "" {
		missing = append(missing, "old_path")
	}
	if strings.TrimSpace(newPath) == "" {
		missing = append(missing, "new_path")
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"a file position needs both old_path and new_path (missing: %s): take them from the file's entry in "+
			"the diff (mr_review.changes_get for a merge request, repository.commit_diff for a commit), which "+
			"gives an added or a deleted file the same path in both",
		strings.Join(missing, ", "),
	)
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
//
// The rules are those of a text position, the only kind GitLab places by a
// line: an image position is judged by [ValidateImagePosition] and a file
// position by [ValidateFilePosition], neither against the diff's lines.
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
