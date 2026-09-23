//go:build e2e

// project_snippet_test.go drives the project snippet builder against the
// stub, whole and through its halves: what it asks GitLab for, and what it
// reads out of GitLab's answer, with the file name taken from the files list
// when the deprecated single-file field is empty.

package fixture

import (
	"net/http"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// TestCreateProjectSnippet_Answers_SendsTheOneFilePrivately checks the
// creator asks for a private snippet carrying the one fixture file under the
// title and description it was given, reads back what GitLab made, and hands
// a refusal back as it came.
func TestCreateProjectSnippet_Answers_SendsTheOneFilePrivately(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/2/snippets",
		stubCreated(map[string]any{"id": 9, "title": "snippet-run", "files": []map[string]any{{"path": projectSnippetFilePath}}}),
		stubRefusal(http.StatusForbidden, "403 Forbidden"))

	got, err := createProjectSnippet(t.Context(), client, 2, "snippet-run", "the World's")
	if err != nil {
		t.Fatalf("createProjectSnippet() error = %v, want nil", err)
	}
	if want := (ProjectSnippet{ID: 9, ProjectID: 2, Title: "snippet-run", FileName: projectSnippetFilePath}); got != want {
		t.Errorf("createProjectSnippet() = %+v, want %+v", got, want)
	}
	sent := stub.recordedRequests()[0].Body
	if sent["title"] != "snippet-run" || sent["description"] != "the World's" || sent["visibility"] != "private" {
		t.Errorf("createProjectSnippet() sent %v, want the title, the description and private", sent)
	}
	files, _ := sent["files"].([]any)
	if len(files) != 1 {
		t.Fatalf("createProjectSnippet() sent files %v, want the one fixture file", sent["files"])
	}
	if file, _ := files[0].(map[string]any); file["file_path"] != projectSnippetFilePath || file["content"] != projectSnippetContent {
		t.Errorf("createProjectSnippet() sent file %v, want the fixture's path and content", file)
	}

	got, err = createProjectSnippet(t.Context(), client, 2, "snippet-run", "the World's")
	if !IsStatus(err, http.StatusForbidden) || got != (ProjectSnippet{}) {
		t.Errorf("createProjectSnippet() on a refusal = %+v, %v; want nothing and GitLab's 403", got, err)
	}
}

// TestNewProjectSnippet_Detached_AsksForItUnderTheRunForThisTest checks the
// project snippet builder whole: asked for in the project it was given, titled
// under the run and described by the test that made it, and handed back as
// GitLab answered it.
func TestNewProjectSnippet_Detached_AsksForItUnderTheRunForThisTest(t *testing.T) {
	stub, e := detachedStub(t)
	stub.answers(http.MethodPost, "/api/v4/projects/2/snippets", stubCreated(map[string]any{"id": 9, "title": "as-answered", "file_name": "answered.txt"}))

	got := NewProjectSnippet(e, Project{ID: 2})

	if want := (ProjectSnippet{ID: 9, ProjectID: 2, Title: "as-answered", FileName: "answered.txt"}); got != want {
		t.Errorf("NewProjectSnippet() = %+v, want %+v", got, want)
	}
	sent := requestTo(t, stub, http.MethodPost, "/api/v4/projects/2/snippets").Body
	if title, _ := sent["title"].(string); !isScopedName(title, "snippet", e) || sent["description"] != "e2e: "+t.Name() {
		t.Errorf("NewProjectSnippet() sent %v, want a title under the run described by this test", sent)
	}
}

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
