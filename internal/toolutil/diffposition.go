package toolutil

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

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
func parseHunkHeader(line string) (oldStart, newStart int) {
	parts := strings.SplitN(line, "@@", 3)
	if len(parts) < 3 {
		return 0, 0
	}
	for r := range strings.FieldsSeq(strings.TrimSpace(parts[1])) {
		if strings.HasPrefix(r, "-") {
			nums := strings.SplitN(r[1:], ",", 2)
			oldStart, _ = strconv.Atoi(nums[0])
		} else if strings.HasPrefix(r, "+") {
			nums := strings.SplitN(r[1:], ",", 2)
			newStart, _ = strconv.Atoi(nums[0])
		}
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
		switch {
		case newLine != 0 && oldLine != 0:
			if dl.Type == LineContext && dl.NewLine == newLine && dl.OldLine == oldLine {
				return nil
			}
		case newLine != 0 && oldLine == 0:
			if dl.Type == LineAdded && dl.NewLine == newLine {
				return nil
			}
			// Detect the common mistake: specifying only new_line for a context line
			if dl.Type == LineContext && dl.NewLine == newLine {
				return fmt.Errorf(
					"new_line %d is an unchanged context line, not an added line — "+
						"for context lines set BOTH old_line=%d and new_line=%d. "+
						"Use new_line alone only for added (+) lines",
					newLine, dl.OldLine, dl.NewLine,
				)
			}
		case oldLine != 0 && newLine == 0:
			if dl.Type == LineRemoved && dl.OldLine == oldLine {
				return nil
			}
			if dl.Type == LineContext && dl.OldLine == oldLine {
				return fmt.Errorf(
					"old_line %d is an unchanged context line, not a removed line — "+
						"for context lines set BOTH old_line=%d and new_line=%d. "+
						"Use old_line alone only for removed (-) lines",
					oldLine, dl.OldLine, dl.NewLine,
				)
			}
		}
	}

	return buildPositionError(diffLines, newLine, oldLine)
}

// buildPositionError constructs a descriptive error when a line is not found
// in the diff at all, listing the valid line ranges.
func buildPositionError(diffLines []DiffLine, newLine, oldLine int) error {
	var minNew, maxNew, minOld, maxOld int
	for _, dl := range diffLines {
		if dl.NewLine != 0 {
			if minNew == 0 || dl.NewLine < minNew {
				minNew = dl.NewLine
			}
			if dl.NewLine > maxNew {
				maxNew = dl.NewLine
			}
		}
		if dl.OldLine != 0 {
			if minOld == 0 || dl.OldLine < minOld {
				minOld = dl.OldLine
			}
			if dl.OldLine > maxOld {
				maxOld = dl.OldLine
			}
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
		"position (%s) is outside the diff range — inline comments can only be placed on lines "+
			"visible in the diff context (valid new_line: %d–%d, valid old_line: %d–%d). "+
			"To comment on code outside the diff, omit the position parameter to create a general discussion instead. "+
			"See https://docs.gitlab.com/api/discussions/#create-a-new-thread-in-the-merge-request-diff",
		target, minNew, maxNew, minOld, maxOld,
	)
}
