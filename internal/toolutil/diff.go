package toolutil

import gl "gitlab.com/gitlab-org/api/client-go/v2"

// DiffOutput represents a single file diff from the GitLab API.
// It is used by both commit diff and repository compare operations.
type DiffOutput struct {
	OldPath     string `json:"old_path"`
	NewPath     string `json:"new_path"`
	AMode       string `json:"a_mode,omitempty"`
	BMode       string `json:"b_mode,omitempty"`
	Diff        string `json:"diff"`
	NewFile     bool   `json:"new_file"`
	RenamedFile bool   `json:"renamed_file"`
	DeletedFile bool   `json:"deleted_file"`
}

// DiffToOutput converts a GitLab API [gl.Diff] to the MCP tool output format.
func DiffToOutput(d *gl.Diff) DiffOutput {
	return DiffOutput{
		OldPath:     d.OldPath,
		NewPath:     d.NewPath,
		AMode:       d.AMode,
		BMode:       d.BMode,
		Diff:        d.Diff,
		NewFile:     d.NewFile,
		RenamedFile: d.RenamedFile,
		DeletedFile: d.DeletedFile,
	}
}
