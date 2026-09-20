// repository_submodules_test.go contains unit tests for the repository submodule MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package repositorysubmodules

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestUpdate_Success verifies Update when success.
func TestUpdate_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `{
			"id": "abc123def456",
			"short_id": "abc123d",
			"title": "Update submodule lib to abc123",
			"author_name": "Dev User",
			"author_email": "dev@example.com",
			"committer_name": "Bot",
			"committer_email": "bot@example.com",
			"authored_date": "2026-01-15T10:29:00Z",
			"message": "Update submodule lib to abc123",
			"parent_ids": ["p1", "p2"],
			"status": "success",
			"created_at": "2026-01-15T10:30:00Z",
			"committed_date": "2026-01-15T10:30:00Z"
		}`)
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Update(t.Context(), client, UpdateInput{
		ProjectID: "42",
		Submodule: "lib/mylib",
		Branch:    "main",
		CommitSHA: "abc123def456",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != "abc123def456" {
		t.Errorf("expected ID 'abc123def456', got %q", out.ID)
	}
	if out.ShortID != "abc123d" {
		t.Errorf("expected short_id 'abc123d', got %q", out.ShortID)
	}
	if out.AuthorName != "Dev User" {
		t.Errorf("expected author_name 'Dev User', got %q", out.AuthorName)
	}
	if out.CommitterName != "Bot" || out.CommitterEmail != "bot@example.com" {
		t.Errorf("expected committer Bot <bot@example.com>, got %q <%q>", out.CommitterName, out.CommitterEmail)
	}
	if out.AuthoredDate == "" {
		t.Error("expected authored_date to be populated")
	}
	if len(out.ParentIDs) != 2 || out.ParentIDs[0] != "p1" {
		t.Errorf("expected parent_ids [p1 p2], got %v", out.ParentIDs)
	}
	if out.Status != "success" {
		t.Errorf("expected status 'success', got %q", out.Status)
	}
}

// TestUpdate_WithCommitMessage verifies that a commit message the caller wrote
// reaches GitLab as commit_message, and that the commit GitLab answered with
// comes back.
//
// The forwarding half is what the name promised and the body did not do: until
// this read the request, the option was a straight-line assignment under a
// guard no assertion could see, so inverting the guard (sending an empty
// message when the caller wrote one, and none when they did not) changed
// nothing any test looked at.
func TestUpdate_WithCommitMessage(t *testing.T) {
	var sent map[string]any
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent = decodeSubmoduleRequestBody(t, r)
		testutil.RespondJSON(w, http.StatusOK, `{
			"id": "abc123",
			"short_id": "abc",
			"title": "Custom message",
			"author_name": "Dev",
			"author_email": "dev@ex.com",
			"message": "Custom message"
		}`)
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Update(t.Context(), client, UpdateInput{
		ProjectID:     "42",
		Submodule:     "lib/mylib",
		Branch:        "main",
		CommitSHA:     "abc123",
		CommitMessage: "Custom message",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSubmoduleSent(t, sent, "commit_message", "Custom message")
	if out.Title != "Custom message" {
		t.Errorf("expected title 'Custom message', got %q", out.Title)
	}
}

// TestUpdate_SendsTheRouteAndTheFieldsThatMoveTheSubmodule pins the request the
// update action builds: the submodule path escaped into the route, and branch
// and commit_sha in the body.
//
// A dropped option is invisible to both quality gates, since an assignment
// carries no branch to flip, and this one is the whole point of the call:
// without commit_sha GitLab is asked to move a submodule nowhere, and the
// handler still reports the commit it answered with as a success.
func TestUpdate_SendsTheRouteAndTheFieldsThatMoveTheSubmodule(t *testing.T) {
	var (
		gotPath string
		sent    map[string]any
	)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPut)
		gotPath = r.URL.EscapedPath()
		sent = decodeSubmoduleRequestBody(t, r)
		testutil.RespondJSON(w, http.StatusOK, `{"id":"c1","short_id":"c1","title":"t","author_name":"A","author_email":"a@t.com","message":"m"}`)
	})

	client := testutil.NewTestClient(t, handler)
	if _, err := Update(t.Context(), client, UpdateInput{
		ProjectID: "42",
		Submodule: "libs/core-module",
		Branch:    "release-3-1",
		CommitSHA: "abc123def456",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if want := "/api/v4/projects/42/repository/submodules/libs%2Fcore-module"; gotPath != want {
		t.Errorf("request path = %q, want %q", gotPath, want)
	}
	assertSubmoduleSent(t, sent, "branch", "release-3-1")
	assertSubmoduleSent(t, sent, "commit_sha", "abc123def456")
	assertSubmoduleNotSent(t, sent, "commit_message")
}

// TestUpdate_Output_CarriesEveryFieldOfTheCommitGitLabSent holds the whole
// output against a response in which no two values agree, so a converter that
// assigns a neighboring field is caught.
//
// Neither gate can see that class: the converter is thirteen assignments and
// one branch each for the three dates, and the existing success test left the
// title and the message carrying one string and the created and committed
// dates another, which is exactly the fixture under which a swap is
// indistinguishable.
func TestUpdate_Output_CarriesEveryFieldOfTheCommitGitLabSent(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id": "abc123def4567890",
			"short_id": "abc123d",
			"title": "Bump core-module",
			"author_name": "Alice Author",
			"author_email": "alice@example.com",
			"committer_name": "Bob Committer",
			"committer_email": "bob@example.com",
			"authored_date": "2026-01-15T10:29:00Z",
			"committed_date": "2026-01-15T10:31:00Z",
			"created_at": "2026-01-15T10:32:00Z",
			"message": "Bump core-module to abc123d\n\nPicked up the parser fix.",
			"parent_ids": ["parent1", "parent2"],
			"status": "running"
		}`)
	})

	client := testutil.NewTestClient(t, handler)
	got, err := Update(t.Context(), client, UpdateInput{
		ProjectID: "42",
		Submodule: "libs/core-module",
		Branch:    "main",
		CommitSHA: "abc123def4567890",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := UpdateOutput{
		ID:             "abc123def4567890",
		ShortID:        "abc123d",
		Title:          "Bump core-module",
		AuthorName:     "Alice Author",
		AuthorEmail:    "alice@example.com",
		AuthoredDate:   time.Date(2026, time.January, 15, 10, 29, 0, 0, time.UTC).String(),
		CommitterName:  "Bob Committer",
		CommitterEmail: "bob@example.com",
		CommittedDate:  time.Date(2026, time.January, 15, 10, 31, 0, 0, time.UTC).String(),
		CreatedAt:      time.Date(2026, time.January, 15, 10, 32, 0, 0, time.UTC).String(),
		Message:        "Bump core-module to abc123d\n\nPicked up the parser fix.",
		ParentIDs:      []string{"parent1", "parent2"},
		Status:         "running",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("update output mismatch:\ngot:  %+v\nwant: %+v", got, want)
	}
}

// decodeSubmoduleRequestBody reads the JSON body the update handler built.
//
// It decodes into a map rather than the SDK option struct because these
// assertions are as much about a key being absent as about its value: the
// options are pointers with omitempty, so "the caller named no commit message"
// and "the caller named an empty one" differ only by the key being on the wire.
// It runs on the mock's goroutine, so it reports and never aborts.
func decodeSubmoduleRequestBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	body := map[string]any{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Errorf("decode request body: %v", err)
		return nil
	}
	return body
}

// assertSubmoduleSent holds one field of the request body to the value the
// caller gave for it.
func assertSubmoduleSent(t *testing.T, body map[string]any, key, want string) {
	t.Helper()
	if got := body[key]; got != want {
		t.Errorf("%s sent = %v, want %q", key, got, want)
	}
}

// assertSubmoduleNotSent holds a field the caller never named to being absent
// rather than present and empty, which is a different instruction: an empty
// commit_message asks GitLab to write a commit with no message at all.
func assertSubmoduleNotSent(t *testing.T, body map[string]any, key string) {
	t.Helper()
	if got, ok := body[key]; ok {
		t.Errorf("%s sent = %v, want the key absent: the caller named no value for it", key, got)
	}
}

// TestUpdate_Error verifies Update when error.
func TestUpdate_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"error"}`)
	})

	client := testutil.NewTestClient(t, handler)
	_, err := Update(t.Context(), client, UpdateInput{
		ProjectID: "42",
		Submodule: "lib/mylib",
		Branch:    "main",
		CommitSHA: "abc123",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFormatUpdateMarkdown pins the card of the commit a submodule update
// created when GitLab answered with little more than the identity.
func TestFormatUpdateMarkdown(t *testing.T) {
	got := renderedText(t, FormatUpdateMarkdown(UpdateOutput{
		ID:         "abc123",
		ShortID:    "abc",
		Title:      "Update submodule",
		AuthorName: "Dev",
	}))

	want := "## Submodule Updated\n\n" +
		"- **Commit**: `abc`\n" +
		"- **Full SHA**: `abc123`\n" +
		"- **Title**: Update submodule\n" +
		"- **Author**: Dev\n" +
		submoduleUpdateHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatUpdateMarkdown_Content pins the whole card of a fully populated
// update. The address is a row of its own rather than an angle-bracketed
// ident, which GFM turns into a mailto autolink to an address nobody chose to
// publish.
func TestFormatUpdateMarkdown_Content(t *testing.T) {
	got := renderedText(t, FormatUpdateMarkdown(UpdateOutput{
		ID:            "abc123def456",
		ShortID:       "abc123d",
		Title:         "Update lib",
		AuthorName:    "Alice",
		AuthorEmail:   "alice@example.com",
		CommittedDate: "2026-03-20T15:45:00Z",
		Message:       "Bump lib to v2",
	}))

	want := "## Submodule Updated\n\n" +
		"- **Commit**: `abc123d`\n" +
		"- **Full SHA**: `abc123def456`\n" +
		"- **Title**: Update lib\n" +
		"- **Author**: Alice\n" +
		"- **Author Email**: alice@example.com\n" +
		"- **Committed**: 20 Mar 2026 15:45 UTC\n" +
		"- **Message**: Bump lib to v2\n" +
		submoduleUpdateHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatUpdateMarkdown_WithStatus pins that the build state GitLab
// reported is a row of the card.
func TestFormatUpdateMarkdown_WithStatus(t *testing.T) {
	got := renderedText(t, FormatUpdateMarkdown(UpdateOutput{
		ID:      "abc123def456",
		ShortID: "abc123d",
		Title:   "Update lib",
		Status:  "success",
	}))

	want := "## Submodule Updated\n\n" +
		"- **Commit**: `abc123d`\n" +
		"- **Full SHA**: `abc123def456`\n" +
		"- **Title**: Update lib\n" +
		"- **Status**: success\n" +
		submoduleUpdateHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestDecorateSubmoduleMeta_UnknownTool verifies the decorator is a no-op for
// a tool not present in submoduleActionMeta, leaving the base options intact.
func TestDecorateSubmoduleMeta_UnknownTool(t *testing.T) {
	options := submoduleOptions("gitlab_unknown_submodule_tool")
	before := options
	decorateSubmoduleMeta(&options, "gitlab_unknown_submodule_tool")
	if options.Usage != before.Usage {
		t.Errorf("Usage should be unchanged for unknown tool, got %q", options.Usage)
	}
	if options.IndividualTool.Description != "" {
		t.Errorf("Description should remain empty for unknown tool, got %q", options.IndividualTool.Description)
	}
}

// TestDecorateSubmoduleMeta_PartialEntry_ReplacesOnlyTheFieldsTheTableFills
// verifies the claim the decorator's own comment makes, field by field: each
// of the four fields is replaced when the entry fills it and left as
// submoduleOptions built it when the entry does not.
//
// Every entry in the real table fills all four, so on every call the suite
// makes all four guards are true and a guard that stopped replacing anything
// would be seen by nothing. The two cases fill disjoint halves so each guard is
// taken both ways. Each entry is installed under a synthetic tool name and
// removed again, since the table is a package-level map.
func TestDecorateSubmoduleMeta_PartialEntry_ReplacesOnlyTheFieldsTheTableFills(t *testing.T) {
	cases := []struct {
		name  string
		entry submoduleActionMetaEntry
	}{
		{name: "usage only", entry: submoduleActionMetaEntry{usage: "Only the usage is filled."}},
		{name: "description only", entry: submoduleActionMetaEntry{description: "Only the description is filled."}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const tool = "gitlab_partial_submodule_tool"
			submoduleActionMeta[tool] = tc.entry
			t.Cleanup(func() { delete(submoduleActionMeta, tool) })

			base := submoduleOptions(tool)
			options := submoduleOptions(tool)
			decorateSubmoduleMeta(&options, tool)

			wantUsage := base.Usage
			if tc.entry.usage != "" {
				wantUsage = tc.entry.usage
			}
			if options.Usage != wantUsage {
				t.Errorf("Usage = %q, want %q", options.Usage, wantUsage)
			}
			if options.IndividualTool.Description != tc.entry.description {
				t.Errorf("Description = %q, want %q", options.IndividualTool.Description, tc.entry.description)
			}
			if !slices.Equal(options.Aliases, base.Aliases) {
				t.Errorf("Aliases = %v, want the base %v: the entry named none", options.Aliases, base.Aliases)
			}
			if !slices.Equal(options.RelatedActions, base.RelatedActions) {
				t.Errorf("RelatedActions = %v, want the base %v: the entry named none", options.RelatedActions, base.RelatedActions)
			}
		})
	}
}

// TestActionSpecs_AliasesAndRelatedActions_AreTheTableEntryNotTheGenericBase
// verifies that what each spec publishes for discovery is the submodule-phrased
// entry rather than the placeholder submoduleOptions starts from.
//
// The counting assertions beside it ("at least two aliases", "some related
// actions") pass just as happily over the base, which names the tool itself and
// two generic repository actions, so a decorator that stopped replacing either
// list served a model the placeholder and nothing failed.
func TestActionSpecs_AliasesAndRelatedActions_AreTheTableEntryNotTheGenericBase(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	byTool := repositorySubmoduleSpecsByTool(t, ActionSpecs(client))

	for tool, meta := range submoduleActionMeta {
		t.Run(tool, func(t *testing.T) {
			spec, ok := byTool[tool]
			if !ok {
				t.Fatalf("%s has discovery metadata but no spec", tool)
			}
			base := submoduleOptions(tool)
			if !slices.Equal(spec.Aliases, meta.aliases) {
				t.Errorf("Aliases = %v, want the table's %v", spec.Aliases, meta.aliases)
			}
			if !slices.Equal(spec.RelatedActions, meta.related) {
				t.Errorf("RelatedActions = %v, want the table's %v", spec.RelatedActions, meta.related)
			}
			if slices.Equal(spec.RelatedActions, base.RelatedActions) {
				t.Errorf("RelatedActions is still the generic base %v", base.RelatedActions)
			}
		})
	}
}

// TestActionSpecs_NonGenericMetadata verifies that every submodule action
// carries non-generic discovery metadata (1:1 audit R-META): a real Usage,
// distinctive submodule-phrased aliases beyond the bare tool name, canonical
// repository.*/commit.* related actions, and a "Returns: … See also: …"
// individual-tool description.
func TestActionSpecs_NonGenericMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	byTool := repositorySubmoduleSpecsByTool(t, ActionSpecs(client))

	for _, tool := range []string{
		"gitlab_list_repository_submodules",
		"gitlab_read_repository_submodule_file",
		"gitlab_update_repository_submodule",
	} {
		t.Run(tool, func(t *testing.T) {
			spec := byTool[tool]
			if strings.Contains(spec.Usage, "Use to execute") || spec.Usage == "" {
				t.Errorf("%s: generic or empty Usage: %q", tool, spec.Usage)
			}
			if len(spec.Aliases) < 2 {
				t.Errorf("%s: expected distinctive aliases, got %v", tool, spec.Aliases)
			}
			for _, a := range spec.Aliases {
				if a == tool {
					t.Errorf("%s: alias must not equal the tool name", tool)
				}
			}
			if len(spec.RelatedActions) == 0 {
				t.Errorf("%s: expected related actions", tool)
			}
			desc := spec.IndividualTool.Description
			if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
				t.Errorf("%s: description must use Returns/See also form, got %q", tool, desc)
			}
		})
	}
}

// TestUpdate_CancelledContext verifies Update when cancelled context.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{ProjectID: "42", Submodule: "lib", Branch: "main", CommitSHA: "abc"})
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}

// TestUpdate_EmptyProjectID verifies Update when empty project ID.
func TestUpdate_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := Update(t.Context(), client, UpdateInput{Submodule: "lib", Branch: "main", CommitSHA: "abc"})
	if err == nil {
		t.Fatal("expected error for empty project_id, got nil")
	}
}

// TestActionSpecs_Metadata verifies canonical metadata for repository submodule actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	byTool := repositorySubmoduleSpecsByTool(t, ActionSpecs(client))

	if len(byTool) != 3 {
		t.Fatalf("len(ActionSpecs) = %d, want 3", len(byTool))
	}
	for _, spec := range byTool {
		if spec.OwnerPackage != "repositorysubmodules" {
			t.Errorf("OwnerPackage for %s = %q, want repositorysubmodules", spec.Name, spec.OwnerPackage)
		}
	}
	for _, name := range []string{"gitlab_list_repository_submodules", "gitlab_read_repository_submodule_file"} {
		t.Run(name, func(t *testing.T) {
			if !byTool[name].ReadOnly || !byTool[name].Idempotent {
				t.Errorf("%s should be read-only and idempotent", name)
			}
		})
	}
	if byTool["gitlab_update_repository_submodule"].ReadOnly || !byTool["gitlab_update_repository_submodule"].Idempotent {
		t.Error("update submodule action should be mutating and idempotent")
	}
}

// TestActionSpecs_UpdateRoute verifies that the update route can be called directly.
func TestActionSpecs_UpdateRoute(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{"id":"c1","short_id":"c1","title":"t","author_name":"A","author_email":"a@t.com","message":"m"}`)
			return
		}
		http.NotFound(w, r)
	}))
	byTool := repositorySubmoduleSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_update_repository_submodule"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "submodule": "lib", "branch": "main", "commit_sha": "abc"})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	if result == nil {
		t.Fatal("Route.Handler returned nil")
	}
}

// repositorySubmoduleSpecsByTool supports repository submodule specs by tool assertions in repositorysubmodules tests.
func repositorySubmoduleSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}
