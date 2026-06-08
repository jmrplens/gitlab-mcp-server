// markdown_test.go contains unit tests for shared Markdown utility functions.
package toolutil

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestBoolPtr verifies that BoolPtr returns a pointer to the expected value.
func TestBoolPtr(t *testing.T) {
	trueVal := BoolPtr(true)
	if trueVal == nil || !*trueVal {
		t.Error("BoolPtr(true) should return a pointer to true")
	}
	falseVal := BoolPtr(false)
	if falseVal == nil || *falseVal {
		t.Error("BoolPtr(false) should return a pointer to false")
	}
}

// TestDiffToOutput verifies that DiffToOutput correctly maps all fields
// from a GitLab Diff to the MCP tool output format.
func TestDiffToOutput(t *testing.T) {
	d := &gl.Diff{
		OldPath:     "old.go",
		NewPath:     "new.go",
		AMode:       "100644",
		BMode:       "100755",
		Diff:        "@@ -1,3 +1,4 @@\n+new line",
		NewFile:     true,
		RenamedFile: false,
		DeletedFile: false,
	}

	out := DiffToOutput(d)

	if out.OldPath != "old.go" {
		t.Errorf("OldPath = %q, want %q", out.OldPath, "old.go")
	}
	if out.NewPath != "new.go" {
		t.Errorf("NewPath = %q, want %q", out.NewPath, "new.go")
	}
	if out.AMode != "100644" {
		t.Errorf("AMode = %q, want %q", out.AMode, "100644")
	}
	if out.BMode != "100755" {
		t.Errorf("BMode = %q, want %q", out.BMode, "100755")
	}
	if !out.NewFile {
		t.Error("NewFile should be true")
	}
	if out.RenamedFile {
		t.Error("RenamedFile should be false")
	}
	if out.DeletedFile {
		t.Error("DeletedFile should be false")
	}
	if !strings.Contains(out.Diff, "+new line") {
		t.Error("Diff should contain the diff content")
	}
}

// TestFormatPagination verifies the compact Markdown pagination string.
func TestFormatPagination(t *testing.T) {
	p := PaginationOutput{Page: 2, TotalPages: 5, TotalItems: 100, PerPage: 20}
	got := FormatPagination(p)
	if !strings.Contains(got, "Page 2 of 5") {
		t.Errorf("FormatPagination should contain page info, got %q", got)
	}
	if !strings.Contains(got, "100 items total") {
		t.Errorf("FormatPagination should contain total items, got %q", got)
	}
}

// TestWritePagination verifies that WritePagination appends pagination to a builder.
func TestWritePagination(t *testing.T) {
	var b strings.Builder
	b.WriteString("header\n")
	p := PaginationOutput{Page: 1, TotalPages: 3, TotalItems: 60, PerPage: 20}
	WritePagination(&b, p)
	got := b.String()
	if !strings.Contains(got, "header") {
		t.Error("WritePagination should preserve existing content")
	}
	if !strings.Contains(got, "Page 1 of 3") {
		t.Errorf("WritePagination should append pagination, got %q", got)
	}
}

// TestMarkdownTableHeader verifies dynamic table header generation.
func TestMarkdownTableHeader(t *testing.T) {
	header := MarkdownTableHeader("ID", "Name", "Status")
	want := "| ID | Name | Status |\n| --- | --- | --- |\n"
	if header != want {
		t.Errorf("MarkdownTableHeader() = %q, want %q", header, want)
	}

	emptyHeader := MarkdownTableHeader()
	if emptyHeader != "" {
		t.Errorf("MarkdownTableHeader() with no columns = %q, want empty", emptyHeader)
	}
}

// TestMarkdownTableSeparator verifies separator generation for arbitrary widths.
func TestMarkdownTableSeparator(t *testing.T) {
	separator := MarkdownTableSeparator(4)
	want := "| --- | --- | --- | --- |\n"
	if separator != want {
		t.Errorf("MarkdownTableSeparator(4) = %q, want %q", separator, want)
	}
	emptySeparator := MarkdownTableSeparator(0)
	if emptySeparator != "" {
		t.Errorf("MarkdownTableSeparator(0) = %q, want empty", emptySeparator)
	}
}

// TestMarkdownTableRow verifies dynamic table row generation.
func TestMarkdownTableRow(t *testing.T) {
	row := MarkdownTableRow("1", "alice", "active")
	want := "| 1 | alice | active |\n"
	if row != want {
		t.Errorf("MarkdownTableRow() = %q, want %q", row, want)
	}

	emptyRow := MarkdownTableRow()
	if emptyRow != "" {
		t.Errorf("MarkdownTableRow() with no cells = %q, want empty", emptyRow)
	}
}

// TestFormatStorageMoveDetailMarkdown verifies the shared storage move detail
// renderer used by group and snippet storage move tools.
func TestFormatStorageMoveDetailMarkdown(t *testing.T) {
	move := StorageMoveMarkdown{
		ID:                     7,
		State:                  "finished",
		SourceStorageName:      "default|primary",
		DestinationStorageName: "storage2",
		CreatedAt:              time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		Entity: &StorageMoveEntityMarkdown{
			Label: "Group",
			Name:  "team|ops",
			URL:   "https://gitlab.example.com/groups/team-ops",
			ID:    42,
		},
	}

	md := FormatStorageMoveDetailMarkdown(move, "Group Storage Move", "Use action 'retrieve_all' to monitor progress")
	for _, want := range []string{
		"## Group Storage Move #7",
		"| **Source** | default&#124;primary |",
		"| **Created** | 2026-01-15 10:30:00 |",
		"| **Group** | [team&#124;ops](https://gitlab.example.com/groups/team-ops) (ID: 42) |",
		"Use action 'retrieve_all' to monitor progress",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatStorageMoveListMarkdown verifies the shared storage move list
// renderer handles empty lists, entity links, and pagination consistently.
func TestFormatStorageMoveListMarkdown(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		md := FormatStorageMoveListMarkdown(nil, StorageMoveListMarkdownOptions{
			Title:        "Snippet Storage Moves",
			EmptyMessage: "No snippet storage moves found.",
			EntityColumn: "Snippet",
		})
		for _, want := range []string{
			"## Snippet Storage Moves",
			"No snippet storage moves found.",
			HintPreserveLinks,
		} {
			if !strings.Contains(md, want) {
				t.Errorf("empty markdown missing %q:\n%s", want, md)
			}
		}
		if strings.Contains(md, "| ID | State |") {
			t.Errorf("empty markdown should not include table:\n%s", md)
		}
	})

	t.Run("with moves", func(t *testing.T) {
		moves := []StorageMoveMarkdown{
			{
				ID:                     1,
				State:                  "finished",
				SourceStorageName:      "default",
				DestinationStorageName: "storage2",
				CreatedAt:              time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
				Entity: &StorageMoveEntityMarkdown{
					Label: "Snippet",
					Name:  "example",
					URL:   "https://gitlab.example.com/snippets/1",
					ID:    55,
				},
			},
			{ID: 2, State: "started"},
		}

		md := FormatStorageMoveListMarkdown(moves, StorageMoveListMarkdownOptions{
			Title:        "Snippet Storage Moves",
			EmptyMessage: "No snippet storage moves found.",
			EntityColumn: "Snippet",
			Pagination:   PaginationOutput{Page: 2},
		})
		for _, want := range []string{
			"| ID | State | Source | Destination | Snippet | Created |",
			"| 1 | finished | default | storage2 | [example](https://gitlab.example.com/snippets/1) | 2026-06-01 12:00:00 |",
			"| 2 | started |",
			"_Page 2, 2 moves shown._",
		} {
			if !strings.Contains(md, want) {
				t.Errorf("list markdown missing %q:\n%s", want, md)
			}
		}
	})
}

// TestNewStorageMoveEntityMarkdown verifies the constructor populates every
// field of the optional entity view model used by storage move tools.
func TestNewStorageMoveEntityMarkdown(t *testing.T) {
	got := NewStorageMoveEntityMarkdown("Group", "team|ops", "https://gitlab.example.com/groups/team-ops", 42)
	if got == nil {
		t.Fatal("expected non-nil entity")
	}
	if got.Label != "Group" || got.Name != "team|ops" ||
		got.URL != "https://gitlab.example.com/groups/team-ops" || got.ID != 42 {
		t.Errorf("entity = %+v, want populated fields", got)
	}
}

// TestNewStorageMoveMarkdown verifies the shared storage move constructor
// populates all fields, including the optional entity reference.
func TestNewStorageMoveMarkdown(t *testing.T) {
	entity := &StorageMoveEntityMarkdown{Label: "Snippet", Name: "example", URL: "https://example.com", ID: 7}
	createdAt := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)

	got := NewStorageMoveMarkdown(99, "finished", "default", "storage2", createdAt, entity)

	if got.ID != 99 || got.State != "finished" ||
		got.SourceStorageName != "default" || got.DestinationStorageName != "storage2" {
		t.Errorf("storage move = %+v, want populated scalar fields", got)
	}
	if !got.CreatedAt.Equal(createdAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, createdAt)
	}
	if got.Entity != entity {
		t.Errorf("Entity = %+v, want %+v", got.Entity, entity)
	}

	// nil entity branch
	none := NewStorageMoveMarkdown(1, "started", "a", "b", createdAt, nil)
	if none.Entity != nil {
		t.Error("Entity should be nil when constructed with nil input")
	}
}

// TestStorageMoveMarkdowns verifies the generic converter produces one
// shared Markdown view model per package-specific input value.
func TestStorageMoveMarkdowns(t *testing.T) {
	type pkgMove struct {
		id    int64
		state string
	}
	inputs := []pkgMove{{id: 1, state: "finished"}, {id: 2, state: "started"}}

	convert := func(pm pkgMove) StorageMoveMarkdown {
		return StorageMoveMarkdown{ID: pm.id, State: pm.state}
	}

	got := StorageMoveMarkdowns(inputs, convert)
	if len(got) != 2 {
		t.Fatalf("got %d markdowns, want 2", len(got))
	}
	if got[0].ID != 1 || got[0].State != "finished" {
		t.Errorf("got[0] = %+v, want {ID:1 State:finished}", got[0])
	}
	if got[1].ID != 2 || got[1].State != "started" {
		t.Errorf("got[1] = %+v, want {ID:2 State:started}", got[1])
	}

	// Empty input slice → empty output slice
	empty := StorageMoveMarkdowns([]pkgMove{}, convert)
	if len(empty) != 0 {
		t.Errorf("empty input = %d items, want 0", len(empty))
	}
}

// TestFormatStorageMoveCollectionMarkdown verifies the generic collection
// renderer maps package-specific moves and delegates to the list renderer
// (empty + populated scenarios).
func TestFormatStorageMoveCollectionMarkdown(t *testing.T) {
	type pkgMove struct {
		id   int64
		name string
	}
	convert := func(pm pkgMove) StorageMoveMarkdown {
		return StorageMoveMarkdown{
			ID:                     pm.id,
			State:                  "finished",
			SourceStorageName:      "default",
			DestinationStorageName: "storage2",
			CreatedAt:              time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
			Entity: &StorageMoveEntityMarkdown{
				Label: "Project", Name: pm.name, URL: "https://example.com/p/" + pm.name, ID: pm.id,
			},
		}
	}

	t.Run("populated", func(t *testing.T) {
		moves := []pkgMove{{id: 1, name: "alpha"}, {id: 2, name: "beta"}}
		md := FormatStorageMoveCollectionMarkdown(
			moves,
			PaginationOutput{Page: 1},
			convert,
			"Project Storage Moves",
			"No project storage moves found.",
			"Project",
		)
		for _, want := range []string{
			"## Project Storage Moves",
			"| ID | State | Source | Destination | Project | Created |",
			"| 1 | finished | default | storage2 | [alpha](https://example.com/p/alpha) | 2026-06-01 12:00:00 |",
			"_Page 1, 2 moves shown._",
		} {
			if !strings.Contains(md, want) {
				t.Errorf("missing %q in:\n%s", want, md)
			}
		}
	})

	t.Run("empty", func(t *testing.T) {
		md := FormatStorageMoveCollectionMarkdown(
			[]pkgMove{},
			PaginationOutput{},
			convert,
			"Project Storage Moves",
			"No project storage moves found.",
			"Project",
		)
		for _, want := range []string{
			"## Project Storage Moves",
			"No project storage moves found.",
		} {
			if !strings.Contains(md, want) {
				t.Errorf("missing %q in:\n%s", want, md)
			}
		}
	})
}

// TestFormatCICDVariableMarkdownEmptyKey verifies the CI/CD variable detail
// renderer returns an empty string when the variable Key is empty.
func TestFormatCICDVariableMarkdownEmptyKey(t *testing.T) {
	md := FormatCICDVariableMarkdown(CICDVariableMarkdown{Key: ""}, CICDVariableMarkdownOptions{Title: "Variable"})
	if md != "" {
		t.Errorf("expected empty string for empty key, got %q", md)
	}
}

// TestFormatCICDVariableMarkdown verifies the shared CI/CD variable detail renderer.
func TestFormatCICDVariableMarkdown(t *testing.T) {
	md := FormatCICDVariableMarkdown(CICDVariableMarkdown{
		Key:              "SECRET_KEY",
		Value:            "hidden-value",
		VariableType:     "env_var",
		Protected:        true,
		Masked:           true,
		Hidden:           true,
		Raw:              true,
		EnvironmentScope: "prod|blue",
		Description:      "token|value",
	}, CICDVariableMarkdownOptions{
		Title:                   "Variable",
		IncludeEnvironmentScope: true,
		Hints:                   []string{"Use action 'update' to change this variable"},
	})

	for _, want := range []string{
		"## Variable: SECRET_KEY",
		"| Protected | " + BoolEmoji(true) + " |",
		"| Hidden | " + BoolEmoji(true) + " |",
		"| Environment Scope | prod&#124;blue |",
		"| Description | token&#124;value |",
		"| Value | [masked] |",
		"Use action 'update' to change this variable",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "hidden-value") {
		t.Errorf("masked variable value leaked in markdown:\n%s", md)
	}
}

// TestCICDVariableMarkdownHelpers verifies shared CI/CD variable constructors
// and collection wrappers used by project, group, and instance variable tools.
func TestCICDVariableMarkdownHelpers(t *testing.T) {
	variable := NewCICDVariableMarkdown("TOKEN", "secret", "file", CICDVariableFlags{Protected: true, Raw: true}, "*", "deploy token")
	variables := CICDVariableMarkdowns([]CICDVariableMarkdown{variable}, func(v CICDVariableMarkdown) CICDVariableMarkdown { return v })
	if len(variables) != 1 || variables[0].Key != "TOKEN" || !variables[0].Protected || !variables[0].Raw {
		t.Fatalf("unexpected mapped variable: %+v", variables)
	}

	detail := FormatCICDVariableDetailMarkdown(variable, "Variable", true)
	for _, want := range []string{"## Variable: TOKEN", "| Value | secret |", "Use action 'delete' to remove this variable"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail markdown missing %q:\n%s", want, detail)
		}
	}

	list := FormatCICDVariableCollectionMarkdown(variables, PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1}, func(v CICDVariableMarkdown) CICDVariableMarkdown { return v }, "Variables", "No variables found.\n", false, "Read a variable")
	for _, want := range []string{"## Variables (1)", "| Key | Type | Protected | Masked |", "| TOKEN | file | " + BoolEmoji(true), "Read a variable"} {
		if !strings.Contains(list, want) {
			t.Errorf("list markdown missing %q:\n%s", want, list)
		}
	}
}

// TestFormatCICDVariableListMarkdown verifies the shared CI/CD variable list renderer.
func TestFormatCICDVariableListMarkdown(t *testing.T) {
	pagination := PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1}
	md := FormatCICDVariableListMarkdown([]CICDVariableMarkdown{
		{Key: "MY|VAR", VariableType: "env_var", Protected: true, EnvironmentScope: "prod|blue"},
	}, pagination, CICDVariableListMarkdownOptions{
		Title:                   "CI/CD Variables",
		EmptyMessage:            "No variables found.\n",
		IncludeEnvironmentScope: true,
		Hints:                   []string{"Use action 'get' with a key to see variable details"},
	})

	for _, want := range []string{
		"## CI/CD Variables (1)",
		"| Key | Type | Protected | Masked | Scope |",
		"| MY&#124;VAR | env_var | " + BoolEmoji(true) + " | " + BoolEmoji(false) + " | prod&#124;blue |",
		"Use action 'get' with a key to see variable details",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}

	empty := FormatCICDVariableListMarkdown(nil, pagination, CICDVariableListMarkdownOptions{EmptyMessage: "No variables found.\n"})
	if empty != "No variables found.\n" {
		t.Errorf("empty markdown = %q, want no-results message", empty)
	}
}

// TestFormatDiscussionListMarkdown verifies shared discussion list rendering.
func TestFormatDiscussionListMarkdown(t *testing.T) {
	body := `literal \n and \t text`
	md := FormatDiscussionListMarkdown([]DiscussionMarkdown{
		NewDiscussionMarkdown("abc123", []DiscussionNoteMarkdown{
			NewDiscussionNoteMarkdown(1, body, "alice", "2026-05-17T12:00:00Z"),
		}),
	}, DiscussionListMarkdownOptions{
		Title:        "Commit Discussions",
		EmptyMessage: "No discussions found.\n",
		Pagination:   PaginationOutput{TotalItems: 42, Page: 1, PerPage: 20, TotalPages: 3},
		Hints:        []string{"Use `gitlab_get_commit_discussion` to view full discussion details"},
	})

	for _, want := range []string{
		"## Commit Discussions (42)",
		"### Discussion abc123",
		"- **@alice** (17 May 2026 12:00 UTC): " + body,
		"Page 1 of 3 | 42 items total | 20 per page",
		"Use `gitlab_get_commit_discussion` to view full discussion details",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}

	empty := FormatDiscussionListMarkdown(nil, DiscussionListMarkdownOptions{EmptyMessage: "No discussions found.\n"})
	if empty != "No discussions found.\n" {
		t.Errorf("empty markdown = %q, want no-results message", empty)
	}
}

// TestDiscussionMarkdownHelpers verifies shared discussion renderers and mapper
// wrappers used by REST and GraphQL discussion tool packages.
func TestDiscussionMarkdownHelpers(t *testing.T) {
	restDiscussion := DiscussionOutput{
		ID: "rest-1",
		Notes: []DiscussionNoteOutput{
			{ID: 11, Body: "hello", Author: "alice", CreatedAt: "2026-05-17T12:00:00Z"},
		},
	}
	renderer := NewDiscussionRenderer("REST Discussions", "No discussions found.\n", "Open a discussion", "Reply to discussion", "Edit note")

	restList := renderer.FormatRESTList(DiscussionOutputMarkdowns([]DiscussionOutput{restDiscussion}), PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1})
	for _, want := range []string{"## REST Discussions (1)", "### Discussion rest-1", "Open a discussion"} {
		if !strings.Contains(restList, want) {
			t.Errorf("REST list markdown missing %q:\n%s", want, restList)
		}
	}

	rendererGraphQLList := renderer.FormatGraphQLList([]DiscussionMarkdown{restDiscussion.MarkdownDiscussion()}, GraphQLPaginationOutput{HasPreviousPage: true, StartCursor: "before"})
	for _, want := range []string{"## REST Discussions (1)", "prev page cursor: `before`", "Open a discussion"} {
		if !strings.Contains(rendererGraphQLList, want) {
			t.Errorf("renderer GraphQL list markdown missing %q:\n%s", want, rendererGraphQLList)
		}
	}

	discussion := restDiscussion.MarkdownDiscussion()
	if got := renderer.FormatDiscussion(discussion); !strings.Contains(got, "Reply to discussion") {
		t.Errorf("discussion markdown missing renderer hint:\n%s", got)
	}
	if got := renderer.FormatNote(restDiscussion.Notes[0].MarkdownNote()); !strings.Contains(got, "Edit note") {
		t.Errorf("note markdown missing renderer hint:\n%s", got)
	}

	graphqlList := FormatGraphQLDiscussionListMarkdown([]DiscussionMarkdown{discussion}, GraphQLPaginationOutput{HasNextPage: true, EndCursor: "cursor"}, func(v DiscussionMarkdown) DiscussionMarkdown { return v }, "GraphQL Discussions", "No discussions found.\n", "Fetch next page")
	for _, want := range []string{"## GraphQL Discussions (1)", "next page cursor: `cursor`", "Fetch next page"} {
		if !strings.Contains(graphqlList, want) {
			t.Errorf("GraphQL list markdown missing %q:\n%s", want, graphqlList)
		}
	}

	restWrapper := FormatRESTDiscussionListMarkdown([]DiscussionOutput{restDiscussion}, PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1}, DiscussionOutput.MarkdownDiscussion, "Wrapped Discussions", "No discussions found.\n", "Wrapped hint")
	if !strings.Contains(restWrapper, "Wrapped hint") {
		t.Errorf("REST wrapper markdown missing hint:\n%s", restWrapper)
	}
}

// TestFormatDiscussionMarkdown verifies shared single discussion rendering.
func TestFormatDiscussionMarkdown(t *testing.T) {
	md := FormatDiscussionMarkdown(NewDiscussionMarkdown("abc123", []DiscussionNoteMarkdown{
		NewDiscussionNoteMarkdown(1, "hello\nworld", "alice", "2026-05-17T12:00:00Z"),
	}), "Use action 'discussion_add_note' to reply to this discussion")

	for _, want := range []string{
		"## Discussion abc123",
		"- **@alice** (17 May 2026 12:00 UTC): hello\nworld",
		"Use action 'discussion_add_note' to reply to this discussion",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatDiscussionNoteMarkdown verifies shared discussion note rendering.
func TestFormatDiscussionNoteMarkdown(t *testing.T) {
	body := "paragraph one\n\n```go\nfmt.Println(\"hi\")\n```"
	md := FormatDiscussionNoteMarkdown(
		NewDiscussionNoteMarkdown(42, body, "alice", "2026-05-17T12:00:00Z"),
		"Use action 'discussion_update_note' with note_id to edit this note",
	)

	for _, want := range []string{
		"## Note",
		"- **ID**: 42",
		"- **Author**: @alice",
		"- **Body**:\n\n> paragraph one",
		"> ```go",
		"> fmt.Println(\"hi\")",
		"- **Created**: 17 May 2026 12:00 UTC",
		"Use action 'discussion_update_note' with note_id to edit this note",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatTemplateListMarkdown verifies shared template list rendering.
func TestFormatTemplateListMarkdown(t *testing.T) {
	pagination := PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1}
	md := FormatTemplateListMarkdown([]TemplateMarkdown{
		NewTemplateMarkdown("Go|Test", "Go template"),
	}, pagination, TemplateListMarkdownOptions{
		Title:        "CI YAML Templates",
		EmptyMessage: "No templates found.\n",
		Hints: []string{
			"Use `gitlab_get_ci_yaml_template` to view a specific template",
			"Use the key to fetch full template content",
		},
	})

	for _, want := range []string{
		"## CI YAML Templates",
		"| Key | Name |",
		"| Go&#124;Test | Go template |",
		"Page 1 of 1 | 1 items total | 20 per page",
		"Use `gitlab_get_ci_yaml_template` to view a specific template",
		"Use the key to fetch full template content",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}

	empty := FormatTemplateListMarkdown(nil, pagination, TemplateListMarkdownOptions{Title: "CI YAML Templates", EmptyMessage: "No templates found.\n"})
	if !strings.Contains(empty, "## CI YAML Templates") || !strings.Contains(empty, "No templates found.") {
		t.Errorf("empty template markdown missing heading or message:\n%s", empty)
	}
}

// TestTemplateMarkdownHelpers verifies shared template renderers and collection
// wrappers used by CI YAML, Dockerfile, and Gitignore template tools.
func TestTemplateMarkdownHelpers(t *testing.T) {
	renderer := NewTemplateRenderer("Templates", "No templates found.\n", "Open a template", "Template", "yaml", "Copy it")
	template := NewTemplateMarkdown("Go", "Go template")
	templates := TemplateMarkdowns([]TemplateMarkdown{template}, func(v TemplateMarkdown) TemplateMarkdown { return v })

	list := renderer.FormatList(templates, PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1})
	for _, want := range []string{"## Templates", "| Go | Go template |", "Open a template"} {
		if !strings.Contains(list, want) {
			t.Errorf("template list markdown missing %q:\n%s", want, list)
		}
	}

	content := renderer.FormatContent("Go", "stages:\n  - test")
	for _, want := range []string{"## Template: Go", "```yaml", "Copy it"} {
		if !strings.Contains(content, want) {
			t.Errorf("template content markdown missing %q:\n%s", want, content)
		}
	}

	collection := FormatTemplateCollectionMarkdown(templates, PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1}, func(v TemplateMarkdown) TemplateMarkdown { return v }, "Collection", "No templates found.\n", "Collection hint")
	if !strings.Contains(collection, "Collection hint") {
		t.Errorf("template collection markdown missing hint:\n%s", collection)
	}
}

// TestFormatTemplateContentMarkdown verifies shared template body rendering.
func TestFormatTemplateContentMarkdown(t *testing.T) {
	md := FormatTemplateContentMarkdown("Dockerfile Template", "Go", "dockerfile", "FROM golang:latest", "Copy this template to your Dockerfile and customize it")

	for _, want := range []string{
		"## Dockerfile Template: Go",
		"```dockerfile\nFROM golang:latest\n```",
		"Copy this template to your Dockerfile and customize it",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}

	withFence := FormatTemplateContentMarkdown(
		"CI YAML Template",
		"Ruby",
		"yaml\n`bad`",
		"script:\n  - echo start\n```\nembedded\n```",
		"first hint",
		"second hint",
	)
	for _, want := range []string{
		"````yamlbad\nscript:",
		"```\nembedded\n```",
		"\n````\n",
		"first hint",
		"second hint",
	} {
		if !strings.Contains(withFence, want) {
			t.Errorf("markdown with embedded fence missing %q:\n%s", want, withFence)
		}
	}

	withLongFence := FormatTemplateContentMarkdown(
		"CI YAML Template",
		"Custom",
		"yaml",
		"script:\n  - echo start\n````\nembedded\n````",
	)
	for _, want := range []string{
		"`````yaml\nscript:",
		"````\nembedded\n````",
		"\n`````\n",
	} {
		if !strings.Contains(withLongFence, want) {
			t.Errorf("markdown with four-backtick fence missing %q:\n%s", want, withLongFence)
		}
	}
}

// TestFormatNoteMarkdown verifies shared GitLab note detail rendering.
func TestFormatNoteMarkdown(t *testing.T) {
	md := FormatNoteMarkdown(
		NewNoteMarkdown(7, "note body", "alice", "2026-05-17T12:00:00Z", NoteMarkdownFlags{System: true, Internal: true, Resolvable: true, Resolved: true}, "bob"),
		NoteMarkdownOptions{
			Title:             "MR Note",
			IncludeInternal:   true,
			IncludeResolvable: true,
			Hints:             []string{"Use note update to edit this note"},
		},
	)

	for _, want := range []string{
		"## MR Note #7",
		"- **Author**: alice",
		"- **Created**: 17 May 2026 12:00 UTC",
		"- **System note**",
		"- **Internal note**",
		"- **Resolvable**: resolved",
		"- **Resolved By**: @bob",
		"note body",
		"Use note update to edit this note",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestNoteMarkdownHelpers verifies shared note mappers and the unresolved note
// branch used by issue and merge request notes.
func TestNoteMarkdownHelpers(t *testing.T) {
	note := NewNoteMarkdown(8, "plain body", "bob", "2026-05-17T12:00:00Z", NoteMarkdownFlags{Resolvable: true}, "")
	notes := NoteMarkdowns([]NoteMarkdown{note}, func(v NoteMarkdown) NoteMarkdown { return v })
	if len(notes) != 1 || notes[0].ID != 8 {
		t.Fatalf("unexpected mapped notes: %+v", notes)
	}

	detail := FormatNoteMarkdown(note, NoteMarkdownOptions{Title: "Issue Note", IncludeResolvable: true})
	for _, want := range []string{"## Issue Note #8", "- **Resolvable**: unresolved", "plain body"} {
		if !strings.Contains(detail, want) {
			t.Errorf("note detail markdown missing %q:\n%s", want, detail)
		}
	}

	list := FormatNoteListMarkdown(notes, PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1}, NoteListMarkdownOptions{Title: "Notes", EmptyMessage: "No notes found.\n"})
	if !strings.Contains(list, "| ID | Author | Created | System |") || strings.Contains(list, "Internal") {
		t.Errorf("note list markdown should omit internal column:\n%s", list)
	}
}

// TestFormatNoteListMarkdown verifies shared GitLab note list rendering.
func TestFormatNoteListMarkdown(t *testing.T) {
	pagination := PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1}
	md := FormatNoteListMarkdown([]NoteMarkdown{
		NewNoteMarkdown(7, "", "alice|dev", "2026-05-17T12:00:00Z", NoteMarkdownFlags{System: true, Internal: true}, ""),
	}, pagination, NoteListMarkdownOptions{
		Title:           "Issue Notes",
		EmptyMessage:    "No issue notes found.\n",
		IncludeInternal: true,
		Hints:           []string{HintPreserveLinks},
	})

	for _, want := range []string{
		"## Issue Notes (1)",
		"| ID | Author | Created | System | Internal |",
		"| 7 | alice&#124;dev | 17 May 2026 12:00 UTC | " + BoolEmoji(true) + " | " + BoolEmoji(true) + " |",
		HintPreserveLinks,
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}

	empty := FormatNoteListMarkdown(nil, pagination, NoteListMarkdownOptions{Title: "Issue Notes", EmptyMessage: "No issue notes found.\n"})
	if !strings.Contains(empty, "## Issue Notes (1)") || !strings.Contains(empty, "No issue notes found.") {
		t.Errorf("empty note markdown missing heading or message:\n%s", empty)
	}
}

// TestMRStateEmoji verifies merge request state emoji mapping.
func TestMRStateEmoji(t *testing.T) {
	tests := []struct {
		state string
		want  string
	}{
		{"opened", "\U0001F7E2"},
		{"merged", "\U0001F7E3"},
		{"closed", "\U0001F534"},
		{"unknown", EmojiQuestion},
		{"", EmojiQuestion},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			if got := MRStateEmoji(tt.state); got != tt.want {
				t.Errorf("MRStateEmoji(%q) = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

// TestIssueStateEmoji verifies issue state emoji mapping.
func TestIssueStateEmoji(t *testing.T) {
	tests := []struct {
		state string
		want  string
	}{
		{"opened", "\U0001F7E2"},
		{"closed", "\U0001F534"},
		{"unknown", EmojiQuestion},
		{"", EmojiQuestion},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			if got := IssueStateEmoji(tt.state); got != tt.want {
				t.Errorf("IssueStateEmoji(%q) = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

// TestPipelineStatusEmoji verifies pipeline status emoji mapping for all statuses.
func TestPipelineStatusEmoji(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"success", "\u2705"},
		{"failed", "\u274C"},
		{"running", "\U0001F535"},
		{"pending", "\U0001F7E1"},
		{"canceled", "\u26D4"},
		{"cancelled", "\u26D4"},
		{"skipped", "\u23ED\uFE0F"},
		{"created", "\U0001F195"},
		{"manual", "\u270B"},
		{"unknown", EmojiQuestion},
		{"", EmojiQuestion},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := PipelineStatusEmoji(tt.status); got != tt.want {
				t.Errorf("PipelineStatusEmoji(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

// TestErrFieldRequired verifies the standard field-required error message.
func TestErrFieldRequired(t *testing.T) {
	err := ErrFieldRequired("project_id")
	if err == nil {
		t.Fatal("ErrFieldRequired should return non-nil error")
	}
	if !strings.Contains(err.Error(), "project_id") {
		t.Errorf("error should mention field name, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error should mention 'required', got %q", err.Error())
	}
}

// TestErrRequiredInt64 verifies the int64 field-required error with parameter guidance.
func TestErrRequiredInt64(t *testing.T) {
	err := ErrRequiredInt64("freeze_period_get", "freeze_period_id")
	if err == nil {
		t.Fatal("ErrRequiredInt64 should return non-nil error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "freeze_period_get") {
		t.Errorf("error should contain operation, got %q", msg)
	}
	if !strings.Contains(msg, "freeze_period_id") {
		t.Errorf("error should contain field name, got %q", msg)
	}
	if !strings.Contains(msg, "must be > 0") {
		t.Errorf("error should mention > 0 constraint, got %q", msg)
	}
}

// TestParseOptionalTime verifies RFC3339 time parsing for optional fields.
func TestParseOptionalTime(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantNil bool
	}{
		{"valid RFC3339", "2026-01-15T10:30:00Z", false},
		{"empty string", "", true},
		{"invalid format", "not-a-date", true},
		{"partial date", "2026-01-15", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseOptionalTime(tt.input)
			if tt.wantNil && got != nil {
				t.Errorf("ParseOptionalTime(%q) = %v, want nil", tt.input, got)
			}
			if !tt.wantNil {
				if got == nil {
					t.Fatalf("ParseOptionalTime(%q) = nil, want non-nil", tt.input)
				}
				expected := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
				if !got.Equal(expected) {
					t.Errorf("ParseOptionalTime(%q) = %v, want %v", tt.input, got, expected)
				}
			}
		})
	}
}

// TestToolResultWithMarkdown verifies the Markdown wrapper produces correct MCP results.
func TestToolResultWithMarkdown(t *testing.T) {
	t.Run("non-empty string", func(t *testing.T) {
		result := ToolResultWithMarkdown("# Hello")
		if result == nil {
			t.Fatal("expected non-nil result for non-empty string")
		}
		if len(result.Content) != 1 {
			t.Fatalf("expected 1 content item, got %d", len(result.Content))
		}
	})
	t.Run("empty string returns nil", func(t *testing.T) {
		result := ToolResultWithMarkdown("")
		if result != nil {
			t.Error("expected nil result for empty string")
		}
	})
}

// TestToolResultAnnotated verifies annotation-aware result creation.
func TestToolResultAnnotated(t *testing.T) {
	t.Run("with annotations", func(t *testing.T) {
		ann := ContentBoth
		result := ToolResultAnnotated("# Hello", ann)
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if len(result.Content) != 1 {
			t.Fatalf("expected 1 content item, got %d", len(result.Content))
		}
		tc, ok := result.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatal("expected TextContent")
		}
		if tc.Annotations == nil {
			t.Error("expected annotations to be set")
		}
		if tc.Annotations.Priority != 0.5 {
			t.Errorf("priority = %v, want 0.5", tc.Annotations.Priority)
		}
	})
	t.Run("nil annotations", func(t *testing.T) {
		result := ToolResultAnnotated("# Hello", nil)
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		tc := result.Content[0].(*mcp.TextContent)
		if tc.Annotations != nil {
			t.Error("expected nil annotations")
		}
	})
	t.Run("empty string returns nil", func(t *testing.T) {
		result := ToolResultAnnotated("", ContentBoth)
		if result != nil {
			t.Error("expected nil result for empty string")
		}
	})
}

type toolResultWithImageTestCase struct {
	name      string
	md        string
	ann       *mcp.Annotations
	imageData []byte
	mimeType  string
	wantText  string
	wantMIME  string
	wantAnn   bool
}

// TestToolResultWithImage_Scenarios_CorrectContent verifies that ToolResultWithImage creates a
// CallToolResult containing both a TextContent with metadata and an
// ImageContent with raw image bytes and MIME type. Covers valid inputs,
// nil annotations, and empty image data to ensure all branches produce
// the expected two-element Content slice.
func TestToolResultWithImage_Scenarios_CorrectContent(t *testing.T) {
	tests := []toolResultWithImageTestCase{
		{
			name:      "valid image with annotations",
			md:        "## Avatar\n\n| Field | Value |\n",
			ann:       ContentDetail,
			imageData: []byte{0x89, 0x50, 0x4E, 0x47},
			mimeType:  "image/png",
			wantText:  "## Avatar\n\n| Field | Value |\n",
			wantMIME:  "image/png",
			wantAnn:   true,
		},
		{
			name:      "nil annotations",
			md:        "# Image",
			ann:       nil,
			imageData: []byte{0xFF, 0xD8, 0xFF},
			mimeType:  "image/jpeg",
			wantText:  "# Image",
			wantMIME:  "image/jpeg",
			wantAnn:   false,
		},
		{
			name:      "empty image data",
			md:        "# Empty",
			ann:       ContentAssistant,
			imageData: []byte{},
			mimeType:  "image/svg+xml",
			wantText:  "# Empty",
			wantMIME:  "image/svg+xml",
			wantAnn:   true,
		},
		{
			name:      "empty markdown text",
			md:        "",
			ann:       ContentBoth,
			imageData: []byte{0x47, 0x49, 0x46},
			mimeType:  "image/gif",
			wantText:  "",
			wantMIME:  "image/gif",
			wantAnn:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertToolResultWithImage(t, tt)
		})
	}
}

func assertToolResultWithImage(t *testing.T, tt toolResultWithImageTestCase) {
	t.Helper()
	result := ToolResultWithImage(tt.md, tt.ann, tt.imageData, tt.mimeType)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Content) != 2 {
		t.Fatalf("expected 2 content items, got %d", len(result.Content))
	}
	assertToolResultImageText(t, result.Content[0], tt.wantText, tt.wantAnn)
	assertToolResultImageContent(t, result.Content[1], tt.imageData, tt.wantMIME)
}

func assertToolResultImageText(t *testing.T, content mcp.Content, wantText string, wantAnn bool) {
	t.Helper()
	textContent, ok := content.(*mcp.TextContent)
	if !ok {
		t.Fatal("first content item should be TextContent")
	}
	if textContent.Text != wantText {
		t.Errorf("TextContent.Text = %q, want %q", textContent.Text, wantText)
	}
	if wantAnn && textContent.Annotations == nil {
		t.Error("expected annotations to be set")
	}
	if !wantAnn && textContent.Annotations != nil {
		t.Errorf("expected nil annotations, got %v", textContent.Annotations)
	}
}

func assertToolResultImageContent(t *testing.T, content mcp.Content, wantData []byte, wantMIME string) {
	t.Helper()
	imageContent, ok := content.(*mcp.ImageContent)
	if !ok {
		t.Fatal("second content item should be ImageContent")
	}
	if imageContent.MIMEType != wantMIME {
		t.Errorf("ImageContent.MIMEType = %q, want %q", imageContent.MIMEType, wantMIME)
	}
	if !bytes.Equal(imageContent.Data, wantData) {
		t.Errorf("ImageContent.Data mismatch: got %v, want %v", imageContent.Data, wantData)
	}
}

// TestAppendResourceLink_NoOp verifies that AppendResourceLink is a no-op
// to prevent JSON-RPC -32002 errors from external HTTP URLs in ResourceLink.
func TestAppendResourceLink_NoOp(t *testing.T) {
	t.Run("does not append to result", func(t *testing.T) {
		result := ToolResultWithMarkdown("# Project")
		AppendResourceLink(result, "https://gitlab.com/project", "My Project", "View in GitLab")
		if len(result.Content) != 1 {
			t.Fatalf("expected 1 content item (no-op), got %d", len(result.Content))
		}
	})
	t.Run("nil result is safe", func(t *testing.T) {
		AppendResourceLink(nil, "https://example.com", "test", "test")
	})
	t.Run("empty URI is safe", func(t *testing.T) {
		result := ToolResultWithMarkdown("# Hello")
		AppendResourceLink(result, "", "test", "test")
		if len(result.Content) != 1 {
			t.Errorf("expected 1 content item, got %d", len(result.Content))
		}
	})
}

// TestFmtMdURL_ClickableLinkFormat verifies that FmtMdURL produces
// a Markdown clickable link [url](url) instead of a plain URL.
func TestFmtMdURL_ClickableLinkFormat(t *testing.T) {
	url := "https://gitlab.example.com/project"
	result := fmt.Sprintf(FmtMdURL, url)
	want := "- **URL**: [https://gitlab.example.com/project](https://gitlab.example.com/project)\n"
	if result != want {
		t.Errorf("FmtMdURL =\n%q\nwant:\n%q", result, want)
	}
}

// TestFmtMdURLNewline_ClickableLinkFormat verifies that FmtMdURLNewline
// produces a clickable link with a leading newline.
func TestFmtMdURLNewline_ClickableLinkFormat(t *testing.T) {
	url := "https://gitlab.example.com/project"
	result := fmt.Sprintf(FmtMdURLNewline, url)
	want := "\n- **URL**: [https://gitlab.example.com/project](https://gitlab.example.com/project)\n"
	if result != want {
		t.Errorf("FmtMdURLNewline =\n%q\nwant:\n%q", result, want)
	}
}

// TestWriteListSummary verifies the "Showing N of M results" summary line
// that is appended for multi-page results and skipped for single-page results.
func TestWriteListSummary(t *testing.T) {
	tests := []struct {
		name  string
		shown int
		p     PaginationOutput
		want  string
	}{
		{
			name:  "multi-page shows summary",
			shown: 20,
			p:     PaginationOutput{Page: 1, TotalPages: 3, TotalItems: 50, PerPage: 20},
			want:  "Showing 20 of 50 results (page 1 of 3)\n\n",
		},
		{
			name:  "single page is no-op",
			shown: 5,
			p:     PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 5, PerPage: 20},
			want:  "",
		},
		{
			name:  "zero total pages is no-op",
			shown: 0,
			p:     PaginationOutput{Page: 0, TotalPages: 0, TotalItems: 0, PerPage: 20},
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			WriteListSummary(&b, tt.shown, tt.p)
			if got := b.String(); got != tt.want {
				t.Errorf("WriteListSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestWriteEmpty verifies the standardized empty-result message.
func TestWriteEmpty(t *testing.T) {
	var b strings.Builder
	WriteEmpty(&b, "merge requests")
	want := "No merge requests found.\n"
	if got := b.String(); got != want {
		t.Errorf("WriteEmpty() = %q, want %q", got, want)
	}
}

// TestBoolEmoji verifies the boolean-to-emoji mapping (✅ for true, ❌ for false).
func TestBoolEmoji(t *testing.T) {
	if got := BoolEmoji(true); got != EmojiSuccess {
		t.Errorf("BoolEmoji(true) = %q, want %q", got, EmojiSuccess)
	}
	if got := BoolEmoji(false); got != EmojiCross {
		t.Errorf("BoolEmoji(false) = %q, want %q", got, EmojiCross)
	}
}

// TestToolResultWithMarkdown_UsesAssistantAnnotation verifies that
// ToolResultWithMarkdown applies ContentAssistant annotations (audience
// "assistant" only) to avoid redundant client display.
func TestToolResultWithMarkdown_UsesAssistantAnnotation(t *testing.T) {
	result := ToolResultWithMarkdown("# Hello")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if tc.Annotations == nil {
		t.Fatal("expected annotations to be set")
	}
	if len(tc.Annotations.Audience) != 1 || tc.Annotations.Audience[0] != "assistant" {
		t.Errorf("audience = %v, want [assistant]", tc.Annotations.Audience)
	}
	if tc.Annotations.Priority != 0.7 {
		t.Errorf("priority = %v, want 0.7", tc.Annotations.Priority)
	}
}

// TestContentAnnotationPresets_Audience verifies that the operation-based
// content annotation presets (ContentList, ContentDetail, ContentMutate)
// all target the "assistant" audience to prevent redundant display.
func TestContentAnnotationPresets_Audience(t *testing.T) {
	tests := []struct {
		name    string
		ann     *mcp.Annotations
		wantPri float64
	}{
		{"ContentList", ContentList, 0.4},
		{"ContentDetail", ContentDetail, 0.6},
		{"ContentMutate", ContentMutate, 0.8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.ann.Audience) != 1 || tt.ann.Audience[0] != "assistant" {
				t.Errorf("%s audience = %v, want [assistant]", tt.name, tt.ann.Audience)
			}
			if tt.ann.Priority != tt.wantPri {
				t.Errorf("%s priority = %v, want %v", tt.name, tt.ann.Priority, tt.wantPri)
			}
		})
	}
}
