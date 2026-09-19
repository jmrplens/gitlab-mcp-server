// action_specs_catalog_test.go judges the canonical action IDs this package
// publishes against the catalog the server really registers.
//
// It is an external test package because the catalog is assembled in
// internal/tools, which imports this one: package mergerequests could not
// reach the oracle without an import cycle. The test exists because nothing
// here did: the commits table invited a model to "use action 'commit.get'",
// and there is no commit domain at all, a single commit being read through
// repository.commit_get. The same invention sat in the commits action's
// related list, beside a "merge_request.changes_get" for a diff the catalog
// registers as mr_review.changes_get.
package mergerequests_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// hintedActionID matches the canonical ID inside a rendered result hint, the
// one form toolutil.HintAction writes: "Use action 'domain.action' to ...".
var hintedActionID = regexp.MustCompile(`Use action '([^']+)'`)

// publishedActionID is one canonical ID this package hands a model, and the
// surface it was read from, so a failure says where to go and fix it.
type publishedActionID struct {
	id     string
	source string
}

// TestPublishedActionIDs_ResolveInTheCatalog asserts that every canonical
// action ID this package publishes names an action the catalog holds.
//
// It compares against the catalog rather than against literals repeated in the
// test, which would only prove a constant equals itself. Each ID is an
// invitation a model acts on, so one no surface registers is a dead end it
// cannot recover from: it is answered "unknown action".
func TestPublishedActionIDs_ResolveInTheCatalog(t *testing.T) {
	registered := catalogActionIDs(t)
	published := publishedActionIDs(t)
	if len(published) == 0 {
		t.Fatal("this package publishes no action ID, so the test judges nothing")
	}

	for _, p := range published {
		t.Run(p.id, func(t *testing.T) {
			if _, ok := registered[p.id]; !ok {
				t.Errorf("%s names action %q, which resolves to no catalog action", p.source, p.id)
			}
		})
	}
}

// publishedActionIDs gathers every canonical ID this package hands a model:
// the related actions its specs carry, and the IDs its formatters quote in
// result hints.
//
// The hint IDs are read back out of rendered text rather than compared with
// the constants they were built from, because what a model acts on is the
// served string: a hint built from the right constant and one built from a
// stale copy of it are indistinguishable at the call site, and this package
// shipped the stale copy in a block of its own.
func publishedActionIDs(t *testing.T) []publishedActionID {
	t.Helper()
	specs := mergerequests.ActionSpecs(testutil.NewTestClient(t, testutil.ForbiddenHandler(t)))
	published := relatedActionIDs(specs)
	return append(published, hintedActionIDs(renderedWithHints()...)...)
}

// renderedWithHints renders every formatter of this package that writes result
// hints. Each list fixture carries one element, since a list formatter answers
// an empty page with a message and returns before the footer its hints are
// written in.
//
// They are listed by hand rather than discovered, so a formatter added with
// hints and not added here is a gap a reader can see. Walking the Markdown
// registry by package path instead would miss the merge request card itself,
// whose output type is an alias for a shape internal/toolutil owns and which
// therefore reports that package as its own.
func renderedWithHints() []string {
	return []string{
		mergerequests.FormatMarkdown(mergerequests.Output{}),
		mergerequests.FormatListMarkdown(mergerequests.ListOutput{MergeRequests: []mergerequests.Output{{}}}),
		mergerequests.FormatApproveMarkdown(mergerequests.ApproveOutput{}),
		mergerequests.FormatCommitsMarkdown(mergerequests.CommitsOutput{Commits: []commits.Output{{}}}),
		mergerequests.FormatPipelinesMarkdown(mergerequests.PipelinesOutput{Pipelines: []pipelines.Output{{}}}),
		mergerequests.FormatRebaseMarkdown(mergerequests.RebaseOutput{}),
		mergerequests.FormatParticipantsMarkdown(mergerequests.ParticipantsOutput{Participants: []mergerequests.ParticipantOutput{{}}}),
		mergerequests.FormatReviewersMarkdown(mergerequests.ReviewersOutput{Reviewers: []mergerequests.ReviewerOutput{{}}}),
		mergerequests.FormatIssuesClosedMarkdown(mergerequests.IssuesClosedOutput{Issues: []issues.BasicOutput{{}}}),
		mergerequests.FormatRelatedIssuesMarkdown(mergerequests.RelatedIssuesOutput{Issues: []issues.BasicOutput{{}}}),
		mergerequests.FormatCreatePipelineMarkdown(pipelines.Output{}),
		mergerequests.FormatTimeStatsMarkdown(mergerequests.TimeStatsOutput{}),
		mergerequests.FormatCreateTodoMarkdown(mergerequests.CreateTodoOutput{}),
		mergerequests.FormatDependencyMarkdown(mergerequests.DependencyOutput{}),
		mergerequests.FormatDependenciesMarkdown(mergerequests.DependenciesOutput{Dependencies: []mergerequests.DependencyOutput{{}}}),
	}
}

// catalogActionIDs builds the canonical catalog at the highest tier and
// returns every action ID it registers. Ultimate is the right tier because it
// is the widest surface: an ID missing from it is missing everywhere, and a
// lower tier would excuse an ID that only a licensed instance holds.
func catalogActionIDs(t *testing.T) map[string]struct{} {
	t.Helper()
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Tier: edition.Ultimate, IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog: %v", err)
	}
	ids := make(map[string]struct{})
	for _, action := range catalog.Actions() {
		ids[string(action.ID)] = struct{}{}
	}
	if len(ids) == 0 {
		t.Fatal("the catalog registered no actions, so this test judges nothing")
	}
	return ids
}

// relatedActionIDs returns the distinct related actions the given specs name.
func relatedActionIDs(specs []toolutil.ActionSpec) []publishedActionID {
	var ids []string
	for _, spec := range specs {
		for _, id := range spec.RelatedActions {
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
	}
	return sourced(ids, "a related action")
}

// hintedActionIDs returns the distinct canonical action IDs quoted in the
// result hints of the given rendered documents.
//
// A token carrying no dot is skipped: that is prose naming a bare action word
// ("use action 'get' with key"), a different claim that cmd/audit_action_ids
// judges on its own terms.
func hintedActionIDs(rendered ...string) []publishedActionID {
	var ids []string
	for _, doc := range rendered {
		for _, match := range hintedActionID.FindAllStringSubmatch(doc, -1) {
			id := match[1]
			if strings.Contains(id, ".") && !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
	}
	return sourced(ids, "a result hint")
}

// sourced labels each ID with where it was read from and sorts them, so the
// subtests run in a stable order.
func sourced(ids []string, source string) []publishedActionID {
	slices.Sort(ids)
	out := make([]publishedActionID, 0, len(ids))
	for _, id := range ids {
		out = append(out, publishedActionID{id: id, source: source})
	}
	return out
}
