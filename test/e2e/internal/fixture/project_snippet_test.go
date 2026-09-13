//go:build e2e

// project_snippet_test.go covers the pure half of the project snippet
// builder: what it reads out of GitLab's answer, with the file name taken
// from the files list when the deprecated single-file field is empty.

package fixture

import (
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// TestProjectSnippetOf_Fields_ReadsWhatATestNeeds checks the conversion
// with the file name in the legacy field and with it only in the files
// list, which is where a snippet created through files carries it.
func TestProjectSnippetOf_Fields_ReadsWhatATestNeeds(t *testing.T) {
	cases := []struct {
		name string
		in   *gl.Snippet
		want ProjectSnippet
	}{
		{
			name: "legacy file name",
			in:   &gl.Snippet{ID: 7, Title: "t", FileName: "e2e.txt"},
			want: ProjectSnippet{ID: 7, ProjectID: 42, Title: "t", FileName: "e2e.txt"},
		},
		{
			name: "file name from the files list",
			in:   &gl.Snippet{ID: 8, Title: "u", Files: []gl.SnippetFile{{Path: "notes.md"}}},
			want: ProjectSnippet{ID: 8, ProjectID: 42, Title: "u", FileName: "notes.md"},
		},
		{
			name: "no file at all",
			in:   &gl.Snippet{ID: 9, Title: "v"},
			want: ProjectSnippet{ID: 9, ProjectID: 42, Title: "v"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := projectSnippetOf(tc.in, 42); got != tc.want {
				t.Errorf("projectSnippetOf() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
