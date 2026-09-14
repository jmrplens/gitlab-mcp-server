//go:build e2e

// search_test.go covers the search group through the server: every scope
// GitLab serves without advanced search is asked for an object the fixture
// built and named uniquely, and the answer is held to that object rather
// than to a count. The scopes that read a repository through Gitaly answer
// as soon as the write is visible, and the ones that read the database
// answer at once; both are asked through a wait, since a loaded Docker
// instance lags on either. The search_type parameter's validation and the
// schema that publishes its values are covered here too.

package common

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/search"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The search waits: the budget the old suite gave every search read.
const (
	searchInterval = 3 * time.Second
	searchWait     = 120 * time.Second
)

// The search_type values the search tools publish, spelled as the schema
// enumerates them and in that order.
var searchTypes = []string{"basic", "advanced", "zoekt"}

// searchTermLength is how much of a fixture's name the scopes whose backend
// matches a substring are asked for.
//
// GitLab refuses a search term it judges abusively long and answers an empty
// list rather than an error, which reads exactly like a scope that found
// nothing. A run measured the boundary by accident: every query of 97
// characters or fewer answered its object, and the one of 100, the
// milestone's own title, answered without it on all three surfaces and
// through two minutes of waiting. The tail is what carries the run's
// identifier and the counter that makes a name unique, so a shorter query is
// still a query for this one object.
const searchTermLength = 40

// searchTail is the last searchTermLength characters of a fixture name.
func searchTail(name string) string {
	if len(name) <= searchTermLength {
		return name
	}
	return name[len(name)-searchTermLength:]
}

// searchCodeTool is the individual tool search.code is declared as, which
// the manifest's individual entries are keyed by and nothing else names:
// an individual entry is a tool and carries no action. It is the name the
// old suite pinned.
const searchCodeTool = "gitlab_search_code"

// searchFixture is a project whose objects each carry a name or a body
// nothing else on the instance does, so a search for it answers that
// object.
type searchFixture struct {
	project   fixture.Project
	issue     fixture.Issue
	mr        fixture.MergeRequest
	milestone fixture.Milestone
	snippet   fixture.Snippet
	// token is the word the committed file, the wiki page, the note and
	// the commit message carry.
	token string
	// commit is the one that wrote the file on the default branch.
	commit fixture.Commit
	// note is the body of the note on the issue.
	note string
	// wikiTitle is the page the wiki repository holds.
	wikiTitle string
}

// buildSearchFixture creates the project and every object the scopes are
// asked for: the file with the token, the issue, the merge request, the
// milestone, the personal snippet, the note on the issue and the wiki page.
func buildSearchFixture(e *harness.Env) searchFixture {
	e.T.Helper()

	project := fixture.NewProject(e, fixture.WithNamePrefix("search"))
	token := e.Name("token")
	commit := fixture.CommitFile(e, project, project.DefaultBranch, "search/"+token+".md", "The token is "+token+".\n", "docs: add the "+token+" file")

	issue := fixture.NewIssue(e, project, e.Name("issue"))
	branch := fixture.NewBranch(e, project, e.Name("mr"))
	fixture.CommitFile(e, project, branch.Name, "search/branch.md", "A change for the merge request.\n", "docs: add the branch file")
	mr := fixture.NewMergeRequest(e, project, branch.Name, project.DefaultBranch, e.Name("mr"))
	milestone := fixture.NewMilestone(e, project, "milestone")
	snippet := fixture.NewSnippet(e)

	note := "A note mentioning " + token + "."
	if _, _, err := e.Client().GL().Notes.CreateIssueNote(project.ID, issue.IID, &gl.CreateIssueNoteOptions{Body: new(note)}, gl.WithContext(e.Ctx)); err != nil {
		e.T.Fatalf("writing the note on issue #%d: %v", issue.IID, err)
	}
	wikiTitle := e.Name("wiki")
	if _, _, err := e.Client().GL().Wikis.CreateWikiPage(project.ID, &gl.CreateWikiPageOptions{
		Title: new(wikiTitle), Content: new("A wiki page mentioning " + token + ".\n"),
	}, gl.WithContext(e.Ctx)); err != nil {
		e.T.Fatalf("writing the wiki page %q: %v", wikiTitle, err)
	}

	return searchFixture{
		project: project, issue: issue, mr: mr, milestone: milestone, snippet: snippet,
		token: token, commit: commit, note: note, wikiTitle: wikiTitle,
	}
}

// TestSearch_EveryScope_FindsTheFixtureObject asks each scope, on every
// surface, for the one object the fixture named for it and finds it.
//
// Replaces: TestIndividual_Search, TestMeta_Search, TestMeta_SearchExtended
func TestSearch_EveryScope_FindsTheFixtureObject(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, buildSearchFixture, func(e *harness.Env, surface harness.Surface, f searchFixture) {
		s := e.On(surface)
		inProject := map[string]any{"project_id": f.project.IDParam()}

		code := harness.Eventually(s, actionSearchCode, withParams(inProject, map[string]any{"query": f.token}), searchInterval, searchWait,
			func(out search.CodeOutput) bool { return blobFound(out, f.commit.FilePath) })
		e.T.Logf("code search for %q found %d blob(s)", f.token, len(code.Blobs))

		projects := harness.Eventually(s, actionSearchProjects, map[string]any{"query": f.project.Name}, searchInterval, searchWait,
			func(out search.ProjectsOutput) bool { return projectFound(out, f.project.ID) })
		e.T.Logf("project search for %q found %d project(s)", f.project.Name, len(projects.Projects))

		issues := harness.Eventually(s, actionSearchIssues, withParams(inProject, map[string]any{"query": f.issue.Title}), searchInterval, searchWait,
			func(out search.IssuesOutput) bool { return issueFound(out, f.issue.IID) })
		e.T.Logf("issue search for %q found %d issue(s)", f.issue.Title, len(issues.Issues))

		mrs := harness.Eventually(s, actionSearchMergeRequests, withParams(inProject, map[string]any{"query": f.mr.Title}), searchInterval, searchWait,
			func(out search.MergeRequestsOutput) bool { return mergeRequestFound(out, f.mr.IID) })
		e.T.Logf("merge request search for %q found %d request(s)", f.mr.Title, len(mrs.MergeRequests))

		commits := harness.Eventually(s, actionSearchCommits, withParams(inProject, map[string]any{"query": f.token}), searchInterval, searchWait,
			func(out search.CommitsOutput) bool { return commitFound(out, f.commit.SHA) })
		e.T.Logf("commit search for %q found %d commit(s)", f.token, len(commits.Commits))

		// The first answer is logged when it does not hold the milestone, so
		// that a run which times out here says what the scope did answer
		// rather than only that it waited.
		milestoneQuery, firstAnswer := searchTail(f.milestone.Title), true
		milestones := harness.Eventually(s, actionSearchMilestones, withParams(inProject, map[string]any{"query": milestoneQuery}), searchInterval, searchWait,
			func(out search.MilestonesOutput) bool {
				if found := milestoneFound(out, f.milestone.ID); found {
					return true
				}
				if firstAnswer {
					firstAnswer = false
					e.T.Logf("the milestone search for %q first answered %d milestone(s), none of them %d: %+v",
						milestoneQuery, len(out.Milestones), f.milestone.ID, out.Milestones)
				}
				return false
			})
		e.T.Logf("milestone search for %q found %d milestone(s)", milestoneQuery, len(milestones.Milestones))

		notes := harness.Eventually(s, actionSearchNotes, withParams(inProject, map[string]any{"query": f.token}), searchInterval, searchWait,
			func(out search.NotesOutput) bool { return noteFound(out, f.note) })
		e.T.Logf("note search for %q found %d note(s)", f.token, len(notes.Notes))

		snippetQuery := searchTail(f.snippet.Title)
		snippets := harness.Eventually(s, actionSearchSnippets, map[string]any{"query": snippetQuery}, searchInterval, searchWait,
			func(out search.SnippetsOutput) bool { return snippetFound(out, f.snippet.ID) })
		e.T.Logf("snippet search for %q found %d snippet(s)", snippetQuery, len(snippets.Snippets))

		username := e.Runtime().Username
		users := harness.Eventually(s, actionSearchUsers, map[string]any{"query": username}, searchInterval, searchWait,
			func(out search.UsersOutput) bool { return userFound(out, username) })
		e.T.Logf("user search for %q found %d user(s)", username, len(users.Users))

		// The wiki scope is held to a row and not to the page: GitLab answers
		// this scope with blob rows (basename, path, data), and client-go
		// decodes them into its wiki page struct, so every field the server
		// publishes here is empty. The row's fields are logged so a run shows
		// what the surface says, and the page-level assertion waits on that
		// fix.
		wiki := harness.Eventually(s, actionSearchWiki, withParams(inProject, map[string]any{"query": f.token}), searchInterval, searchWait,
			func(out search.WikiOutput) bool { return len(out.WikiBlobs) > 0 })
		e.T.Logf("wiki search for %q found %d page(s); the first reads %+v (the page is %q)", f.token, len(wiki.WikiBlobs), wiki.WikiBlobs[0], f.wikiTitle)
	})
}

// blobFound reports whether a code search holds the file at the path.
func blobFound(out search.CodeOutput, path string) bool {
	for _, blob := range out.Blobs {
		if blob.Path == path || blob.Filename == path {
			return true
		}
	}
	return false
}

// projectFound reports whether a project search holds the project.
func projectFound(out search.ProjectsOutput, id int64) bool {
	for _, project := range out.Projects {
		if project.ID == id {
			return true
		}
	}
	return false
}

// issueFound reports whether an issue search holds the issue.
func issueFound(out search.IssuesOutput, iid int64) bool {
	for _, issue := range out.Issues {
		if issue.IID == iid {
			return true
		}
	}
	return false
}

// mergeRequestFound reports whether a merge request search holds the
// request.
func mergeRequestFound(out search.MergeRequestsOutput, iid int64) bool {
	for _, mr := range out.MergeRequests {
		if mr.IID == iid {
			return true
		}
	}
	return false
}

// commitFound reports whether a commit search holds the commit.
func commitFound(out search.CommitsOutput, sha string) bool {
	for _, commit := range out.Commits {
		if commit.ID == sha {
			return true
		}
	}
	return false
}

// milestoneFound reports whether a milestone search holds the milestone.
func milestoneFound(out search.MilestonesOutput, id int64) bool {
	for _, milestone := range out.Milestones {
		if milestone.ID == id {
			return true
		}
	}
	return false
}

// noteFound reports whether a note search holds a note with the body.
func noteFound(out search.NotesOutput, body string) bool {
	for _, note := range out.Notes {
		if note.Body == body {
			return true
		}
	}
	return false
}

// snippetFound reports whether a snippet search holds the snippet.
func snippetFound(out search.SnippetsOutput, id int64) bool {
	for _, snippet := range out.Snippets {
		if snippet.ID == id {
			return true
		}
	}
	return false
}

// userFound reports whether a user search holds the user.
func userFound(out search.UsersOutput, username string) bool {
	for _, user := range out.Users {
		if user.Username == username {
			return true
		}
	}
	return false
}

// TestSearchType_Basic_IsAcceptedAndAnswers asks the code scope for the
// fixture file with search_type set to basic on every surface, which every
// instance serves, and finds the file.
//
// Replaces: TestSearchType_BasicSearchWorks
func TestSearchType_Basic_IsAcceptedAndAnswers(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) searchFixture {
		project := fixture.NewProject(e, fixture.WithNamePrefix("searchtype"))
		token := e.Name("token")
		commit := fixture.CommitFile(e, project, project.DefaultBranch, "search/"+token+".md", "The token is "+token+".\n", "docs: add the "+token+" file")
		return searchFixture{project: project, token: token, commit: commit}
	}, func(e *harness.Env, surface harness.Surface, f searchFixture) {
		s := e.On(surface)

		code := harness.Eventually(s, actionSearchCode, map[string]any{
			"project_id": f.project.IDParam(), "query": f.token, "search_type": "basic",
		}, searchInterval, searchWait, func(out search.CodeOutput) bool { return blobFound(out, f.commit.FilePath) })
		e.T.Logf("basic code search for %q found %d blob(s)", f.token, len(code.Blobs))
	})
}

// TestSearchType_InvalidValue_IsRefusedNamingTheChoices sends a
// search_type no backend is called on every surface and reads a refusal
// that names the parameter and the three values it takes. Who refuses
// depends on the surface, and both answers are held to the same wording:
// the individual tool's schema enumerates the values, so the SDK refuses
// the argument before the handler runs, while the dispatchers validate
// only the required parameters and the handler refuses the value itself.
//
// Replaces: TestSearchType_InvalidValueFails
func TestSearchType_InvalidValue_IsRefusedNamingTheChoices(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		params := map[string]any{"query": "anything", "search_type": "semantic"}

		var refusal string
		if surface == harness.SurfaceIndividual {
			refusal = harness.Refused(s, actionSearchCode, params, harness.FailureInvalidParams)
		} else {
			refusal = harness.ExpectToolError(s, actionSearchCode, params, "search_type")
		}
		assertMentions(e, "the refusal of search_type=semantic", refusal, append([]string{"search_type"}, searchTypes...)...)
	})
}

// TestSearchType_PublishedSchema_EnumeratesTheValues reads the code
// scope's entry of the tools manifest on every surface and checks the
// input schema it publishes constrains search_type to the three values, in
// order: the schema is what a client validates against before calling, and
// a value dropped from it would only be refused on the wire.
//
// Replaces: TestSearchType_SchemasExposeEnum
func TestSearchType_PublishedSchema_EnumeratesTheValues(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)

		detail := readToolManifestEntry(e, s, string(actionSearchCode), searchCodeTool)
		schema := schemaObject(e, detail.InputSchema)
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			e.T.Fatalf("the schema of %s publishes no properties: %v", detail.ID, schema)
		}
		property, ok := properties["search_type"].(map[string]any)
		if !ok {
			e.T.Fatalf("the schema of %s publishes no search_type: %v", detail.ID, properties)
		}
		got := enumValues(property["enum"])
		if strings.Join(got, ",") != strings.Join(searchTypes, ",") {
			e.T.Errorf("the schema of %s enumerates search_type as %v, want %v", detail.ID, got, searchTypes)
		}
	})
}

// readToolManifestEntry reads the detail resource of the manifest entry
// that runs an action on this session's surface, and then its own detail
// URI. The dispatcher surfaces' entries name the action they run; the
// individual surface's name only the tool, so the tool the action is
// declared as is given too.
func readToolManifestEntry(e *harness.Env, s *harness.Session, action, individualTool string) resources.ToolSurfaceDetail {
	e.T.Helper()

	manifest := readToolsManifest(e, s)
	entry, found := manifestEntryForAction(manifest, action, individualTool)
	if !found {
		e.T.Fatalf("the %s manifest has no entry running %s among %d entries", s.Surface(), action, len(manifest.Entries))
	}
	uri := entry.DetailURI
	if uri == "" {
		uri = toolsManifestURI + "/" + entry.ID
	}
	result := s.ReadResource(uri)
	if len(result.Contents) == 0 {
		e.T.Fatalf("%s answered with no contents", uri)
	}
	var detail resources.ToolSurfaceDetail
	if err := json.Unmarshal([]byte(result.Contents[0].Text), &detail); err != nil {
		e.T.Fatalf("decoding %s: %v", uri, err)
	}
	return detail
}

// The entry kinds the manifest uses, spelled as the resource spells them.
// The two dispatcher kinds are named in mcp_tiers_test.go.
const manifestKindIndividualTool = "individual_tool"

// manifestEntryForAction finds the manifest entry that runs the canonical
// action: on the dynamic surface an entry's action is the canonical ID, on
// the meta surface its domain and action name make the ID up, and on the
// individual surface an entry is a tool and names no action, so it is the
// one whose ID is the tool the action is declared as.
func manifestEntryForAction(manifest resources.ToolSurfaceManifest, action, individualTool string) (resources.ToolSurfaceEntry, bool) {
	domain, name, _ := strings.Cut(action, ".")
	for _, entry := range manifest.Entries {
		switch entry.Kind {
		case manifestKindDynamicAction:
			if entry.Action == action {
				return entry, true
			}
		case manifestKindMetaAction:
			if entry.Domain == domain && entry.Action == name {
				return entry, true
			}
		case manifestKindIndividualTool:
			if entry.ID == individualTool {
				return entry, true
			}
		}
	}
	return resources.ToolSurfaceEntry{}, false
}

// schemaObject reads a published schema as the map a JSON document is.
func schemaObject(e *harness.Env, raw any) map[string]any {
	e.T.Helper()
	encoded, err := json.Marshal(raw)
	if err != nil {
		e.T.Fatalf("re-encoding the schema: %v", err)
	}
	var schema map[string]any
	if err = json.Unmarshal(encoded, &schema); err != nil {
		e.T.Fatalf("decoding the schema: %v", err)
	}
	return schema
}

// enumValues reads the string values of a schema enum.
func enumValues(raw any) []string {
	values, isList := raw.([]any)
	if !isList {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text, isText := value.(string); isText {
			out = append(out, text)
		}
	}
	return out
}
