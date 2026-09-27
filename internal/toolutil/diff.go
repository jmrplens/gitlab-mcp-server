package toolutil

import (
	"fmt"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// DiffOutput represents a single file diff from the GitLab API.
// It is used by both commit diff and repository compare operations.
//
// The last three flags are what lib/api/entities/diff.rb sends beside the
// eight client-go's Diff models, read from the captured response (see
// [DiffExtra]); they are spelled and tagged the way a merge request's file
// diff publishes them.
type DiffOutput struct {
	OldPath       string `json:"old_path"`
	NewPath       string `json:"new_path"`
	AMode         string `json:"a_mode,omitempty"`
	BMode         string `json:"b_mode,omitempty"`
	Diff          string `json:"diff"`
	NewFile       bool   `json:"new_file"`
	RenamedFile   bool   `json:"renamed_file"`
	DeletedFile   bool   `json:"deleted_file"`
	GeneratedFile bool   `json:"generated_file"`
	Collapsed     bool   `json:"collapsed,omitempty"`
	TooLarge      bool   `json:"too_large,omitempty"`
}

// Status names what happened to the file in the diff, and says so when GitLab
// sent no diff text for it, having collapsed it or found it too large, or
// marked the file generated: "modified (too large)" tells a reader why the
// diff is empty where "modified" alone reads as a change with nothing in it.
func (d DiffOutput) Status() string {
	status := "modified"
	switch {
	case d.NewFile:
		status = "added"
	case d.DeletedFile:
		status = "deleted"
	case d.RenamedFile:
		status = "renamed"
	}
	var notes []string
	if d.Collapsed {
		notes = append(notes, "collapsed")
	}
	if d.TooLarge {
		notes = append(notes, "too large")
	}
	if d.GeneratedFile {
		notes = append(notes, "generated")
	}
	if len(notes) == 0 {
		return status
	}
	return status + " (" + strings.Join(notes, ", ") + ")"
}

// DiffExtra is what Entities::Diff sends on a file diff that client-go's Diff
// does not model: whether GitLab collapsed the diff or left it out as too
// large, which is why a diff can arrive empty, and whether the file is
// generated. All three are exposed with no condition. The gap is recorded in
// docs/development/upstream-bugs.md.
type DiffExtra struct {
	Collapsed     bool `json:"collapsed"`
	TooLarge      bool `json:"too_large"`
	GeneratedFile bool `json:"generated_file"`
}

// DiffToOutput converts a GitLab API [gl.Diff] to the MCP tool output format,
// with the flags the captured response carried beside it.
func DiffToOutput(d *gl.Diff, extra DiffExtra) DiffOutput {
	return DiffOutput{
		OldPath:       d.OldPath,
		NewPath:       d.NewPath,
		AMode:         d.AMode,
		BMode:         d.BMode,
		Diff:          d.Diff,
		NewFile:       d.NewFile,
		RenamedFile:   d.RenamedFile,
		DeletedFile:   d.DeletedFile,
		GeneratedFile: extra.GeneratedFile,
		Collapsed:     extra.Collapsed,
		TooLarge:      extra.TooLarge,
	}
}

// CapturedDiffs reads the flags off a captured list of file diffs, one per
// diff in order, the count held to what the SDK decoded.
func CapturedDiffs(capture *gitlabclient.ResponseCapture, decoded int) ([]DiffExtra, error) {
	return capturedList[DiffExtra](capture, decoded, "diffs")
}

// CapturedCompareDiffs reads the same off a captured comparison, whose file
// diffs sit under diffs beside the commits.
func CapturedCompareDiffs(capture *gitlabclient.ResponseCapture, decoded int) ([]DiffExtra, error) {
	compared, err := capturedOne[struct {
		Diffs []DiffExtra `json:"diffs"`
	}](capture)
	if err != nil {
		return nil, err
	}
	if len(compared.Diffs) != decoded {
		return nil, fmt.Errorf("the captured answer holds %d diffs and the SDK decoded %d", len(compared.Diffs), decoded)
	}
	return compared.Diffs, nil
}
