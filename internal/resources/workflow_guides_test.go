// workflow_guides_test.go holds the guards over the five static guide bodies
// registered by [RegisterWorkflowGuides]. The bodies are text/markdown a client
// reads straight out of resources/read: no formatter, no registry and no schema
// sits between them and the model, so a call they name wrongly is a call the
// model makes wrongly, and markup they write by hand is markup the page
// renders.
package resources

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
)

// guideToolNamePattern matches the gitlab_* MCP tool names a guide body cites.
var guideToolNamePattern = regexp.MustCompile(`gitlab_[a-z0-9_]+`)

// guideCodeSpanPattern matches one single-line code span. It cannot match a
// fence line, which is a run of backticks with nothing between them, and it
// cannot reach across a line break, so a fenced body is left alone too.
var guideCodeSpanPattern = regexp.MustCompile("`[^`\n]+`")

// guideFencePattern matches a code fence line: a run of at least three
// backticks, optionally followed by an info string.
var guideFencePattern = regexp.MustCompile("^`{3,}[a-zA-Z0-9]*$")

// guideActionIDPattern matches a canonical "domain.action" catalog ID, the
// spelling every tool surface can reach.
var guideActionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$`)

// guideActionIDs returns every canonical action ID the guide bodies name in a
// code span, deduplicated and sorted.
//
// Only code spans are read. The pipeline guide names the MCP method
// "resources.subscribe" in running prose, which has the shape of an action ID
// and is not one, and a scan of the prose would report it as a catalog miss.
func guideActionIDs() []string {
	var ids []string
	for _, guide := range workflowGuides {
		for _, span := range guideCodeSpanPattern.FindAllString(guide.content, -1) {
			id := strings.Trim(span, "`")
			if guideActionIDPattern.MatchString(id) && !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
	}
	slices.Sort(ids)
	return ids
}

// TestWorkflowGuides_ToolNames_AreOnlyTheDefaultSurfaceExecutor verifies that
// the only MCP tool name the five guide bodies mention is the dynamic
// surface's executor.
//
// How: every gitlab_* token of every body is collected, deduplicated, sorted,
// and the whole set compared with the one name expected.
//
// Expected: exactly [gitlab_execute_action], and nothing else.
//
// Why: one body is served whatever GITLAB_MCP_TOOL_SURFACE says, and the
// default dynamic surface registers two tools. The pipeline guide used to name
// five individual-surface tools (gitlab_pipeline_get, gitlab_job_list,
// gitlab_job_trace, gitlab_ci_variable_list, gitlab_job_retry), so a model on
// the default surface was handed five tool calls its own tools/list never
// carried. The executor is named because the guide has to say what an action ID
// is passed to; any second name here is that defect coming back.
func TestWorkflowGuides_ToolNames_AreOnlyTheDefaultSurfaceExecutor(t *testing.T) {
	var found []string
	for _, guide := range workflowGuides {
		for _, name := range guideToolNamePattern.FindAllString(guide.content, -1) {
			if !slices.Contains(found, name) {
				found = append(found, name)
			}
		}
	}
	slices.Sort(found)

	want := []string{dynamictools.ExecuteActionToolName}
	if !slices.Equal(found, want) {
		t.Errorf("guide bodies name tools %v, want %v", found, want)
	}
}

// TestWorkflowGuides_ActionIDsResolve_InTheCanonicalCatalog verifies that every
// canonical action ID the guide bodies name is an action the catalog carries.
//
// How: the IDs are read out of the bodies' code spans, the whole set is
// compared with the five the pipeline guide is expected to name, and each is
// looked up in the Ultimate-tier catalog every surface is projected from.
//
// Expected: exactly the five diagnostic steps, each resolving to an action.
//
// Why: an ID is only portable while it exists. Nothing else reads these bodies,
// so a renamed action would leave the guide naming a call that no surface can
// dispatch, exactly as the individual tool names it replaced did.
func TestWorkflowGuides_ActionIDsResolve_InTheCanonicalCatalog(t *testing.T) {
	found := guideActionIDs()
	want := []string{"ci_variable.list", "job.list", "job.retry", "job.trace", "pipeline.get"}
	if !slices.Equal(found, want) {
		t.Fatalf("guide bodies name action IDs %v, want %v", found, want)
	}

	catalog := fullSurfaceCatalog(t)
	for _, id := range found {
		t.Run(id, func(t *testing.T) {
			if _, ok := catalog.Action(actioncatalog.ActionID(id)); !ok {
				t.Errorf("guide names action %q, which the canonical catalog does not carry", id)
			}
		})
	}
}

// TestWorkflowGuides_Bodies_KeepMarkupInsideAFenceOrCodeSpan verifies that each
// body closes every code fence it opens and writes no angle bracket outside a
// fence or a code span.
//
// How: each body is walked line by line, a fence line toggles the fence state,
// and outside a fence the line's code spans are removed before it is searched
// for "<".
//
// Expected: no unclosed fence and no bare angle bracket in any of the five
// bodies.
//
// Why: the placeholders these guides are made of (<ticket>, <type>, <scope>)
// render as a raw tag the moment one lands in ordinary prose, which is the
// runtime hostile-value rule applied to a body no formatter registry drives.
// The fence half is the other side of the same containment: the two blocks in
// the conventional-commits guide are written by toolutil.MarkdownFencedBlock,
// which sizes the fence to the body, and an unbalanced fence would render the
// rest of the guide as the content of a code block.
func TestWorkflowGuides_Bodies_KeepMarkupInsideAFenceOrCodeSpan(t *testing.T) {
	for _, guide := range workflowGuides {
		t.Run(guide.name, func(t *testing.T) {
			inFence := false
			for i, line := range strings.Split(guide.content, "\n") {
				if guideFencePattern.MatchString(line) {
					inFence = !inFence
					continue
				}
				if inFence {
					continue
				}
				if bare := guideCodeSpanPattern.ReplaceAllString(line, ""); strings.Contains(bare, "<") {
					t.Errorf("line %d writes an angle bracket outside a fence and outside a code span: %q", i+1, line)
				}
			}
			if inFence {
				t.Error("a code fence is opened and never closed")
			}
		})
	}
}
