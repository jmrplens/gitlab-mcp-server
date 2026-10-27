// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package tools

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestRegisterMetaCatalog_NilInputs verifies that each of the three nil
// combinations registers nothing and does not panic.
//
// Meta catalog registration is called from configurable startup paths, so a
// partial setup must not take the process down. Nothing inside the function
// answers for that: there is no guard in front of its loop, because either nil
// is already refused one call down and an early return here could therefore
// not be observed. That refusal is the whole contract, and this is the only
// thing holding it.
//
// The third combination is the one that earns its place, and it is the one
// this test lacked. A nil server with a real catalog is the only shape that
// walks the loop at all, so it is the only one that would notice if
// AddMetaTool ever stopped refusing a nil server; the other two never reach
// the loop body, because Groups() answers nil for a nil catalog.
func TestRegisterMetaCatalog_NilInputs(t *testing.T) {
	RegisterMetaCatalog(nil, nil)

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, &mcp.ServerOptions{SchemaCache: testSchemaCache})
	RegisterMetaCatalog(server, nil)
	if names := toolNamesFromServer(t, server); len(names) != 0 {
		t.Errorf("registered tools = %v, want none from a missing catalog", names)
	}

	// A catalog carrying a group, so the loop body runs rather than being
	// skipped by an empty group list: a nil server has to be refused by
	// AddMetaTool for this to return at all.
	catalog := actioncatalog.NewCatalog()
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_nil_server_probe"})
	group.SetAction(actioncatalog.Action{
		Name:  "get",
		Route: toolutil.Route(func(context.Context, map[string]any) (any, error) { return map[string]any{}, nil }),
	})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}
	RegisterMetaCatalog(nil, catalog)
}

// TestRegisterMetaCatalog_GroupFormatterRendersTheResult pins that a group's
// own formatter is what its registered dispatcher renders results with, and
// that the shared Markdown dispatcher only fills in for a group that declares
// none.
//
// Neither direction fails loudly. A group whose formatter is dropped still
// answers, in the shared dispatcher's rendering rather than its own; a group
// left with no formatter at all still answers too, because MakeMetaHandler
// falls back to a JSON one of its own, which is not the Markdown every other
// tool of this server serves.
func TestRegisterMetaCatalog_GroupFormatterRendersTheResult(t *testing.T) {
	const sentinel = "rendered by the group's own formatter"

	spec := toolutil.NewActionSpec("list", toolutil.RouteAction(nil,
		func(_ context.Context, _ *gitlabclient.Client, _ struct{}) (struct{}, error) {
			return struct{}{}, nil
		}), toolutil.ActionSpecOptions{
		ReadOnly:     true,
		OwnerPackage: "tools",
	})
	group, err := actioncatalog.GroupFromSpecs(actioncatalog.GroupOptions{
		ToolName:     "gitlab_test_formatter",
		Title:        "Formatter probe",
		Description:  "Formatter probe group.",
		OwnerPackage: "tools",
		SurfaceKind:  actioncatalog.SurfaceKindMetaGroup,
		ReadOnly:     true,
		FormatResult: func(any) *mcp.CallToolResult {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: sentinel}}}
		},
	}, []toolutil.ActionSpec{spec})
	if err != nil {
		t.Fatalf("GroupFromSpecs() error = %v", err)
	}
	catalog := actioncatalog.NewCatalog()
	if addErr := catalog.AddGroup(group); addErr != nil {
		t.Fatalf("AddGroup() error = %v", addErr)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, &mcp.ServerOptions{SchemaCache: testSchemaCache})
	RegisterMetaCatalog(server, catalog)

	session := connectServerForTools(t, server)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "gitlab_test_formatter",
		Arguments: map[string]any{"action": "list"},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() IsError = true: %#v", result.Content)
	}
	if len(result.Content) == 0 {
		t.Fatal("the dispatcher returned no content at all")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] = %T, want TextContent", result.Content[0])
	}
	if !strings.Contains(text.Text, sentinel) {
		t.Errorf("content = %q, want the group's declared formatter to have rendered it", text.Text)
	}
}

// hintProbeCatalog builds a catalog of one group, gitlab_test_hints, whose
// list action's formatter writes two next steps through HintAction: one to the
// group's own get action and one to an ID no catalog holds. The individual
// tools are gitlab_test_hints_list and gitlab_test_hints_get.
func hintProbeCatalog(t *testing.T) *actioncatalog.Catalog {
	t.Helper()
	var getID string
	route := toolutil.RouteAction(nil, func(_ context.Context, _ *gitlabclient.Client, _ struct{}) (struct{}, error) {
		return struct{}{}, nil
	})
	spec := func(name string) toolutil.ActionSpec {
		return toolutil.NewActionSpec(name, route, toolutil.ActionSpecOptions{
			ReadOnly:       true,
			OwnerPackage:   "tools",
			IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_test_hints_" + name, Title: "Hints " + name, Description: "Hint probe."},
		})
	}
	group, err := actioncatalog.GroupFromSpecs(actioncatalog.GroupOptions{
		ToolName:     "gitlab_test_hints",
		Title:        "Hint probe",
		Description:  "Hint probe group.",
		OwnerPackage: "tools",
		SurfaceKind:  actioncatalog.SurfaceKindMetaGroup,
		ReadOnly:     true,
		FormatResult: func(any) *mcp.CallToolResult {
			var b strings.Builder
			b.WriteString("## Probe\n")
			toolutil.WriteHints(&b, toolutil.HintAction(getID, "read one"), toolutil.HintAction("other.missing", "do something else"))
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: b.String()}}}
		},
	}, []toolutil.ActionSpec{spec("list"), spec("get")})
	if err != nil {
		t.Fatalf("GroupFromSpecs() error = %v", err)
	}
	catalog := actioncatalog.NewCatalog()
	if addErr := catalog.AddGroup(group); addErr != nil {
		t.Fatalf("AddGroup() error = %v", addErr)
	}
	for _, action := range catalog.Actions() {
		if action.Name == "get" {
			getID = string(action.ID)
		}
	}
	if getID == "" {
		t.Fatal("the probe catalog holds no get action")
	}
	return catalog
}

// callForText calls tool on server with arguments and returns the text of the
// first content block.
func callForText(t *testing.T, server *mcp.Server, tool string, arguments map[string]any) string {
	t.Helper()
	result, err := connectServerForTools(t, server).CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: arguments})
	if err != nil {
		t.Fatalf("CallTool(%s) error = %v", tool, err)
	}
	if result.IsError || len(result.Content) == 0 {
		t.Fatalf("CallTool(%s) = %#v, want a result", tool, result)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] = %T, want TextContent", result.Content[0])
	}
	return text.Text
}

// TestRegisterMetaCatalog_NextSteps_NameTheMetaCall verifies that a next step
// a meta tool serves names the action the way the meta surface calls it, the
// group's tool with the action's name within it, and that a step naming an
// action the catalog does not hold is left as it was written (issue 1325).
//
// A meta tool's action argument refuses a canonical ID, so the hint the
// formatter wrote, "Use action '<id>'", was a call the model could not make.
func TestRegisterMetaCatalog_NextSteps_NameTheMetaCall(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, &mcp.ServerOptions{SchemaCache: testSchemaCache})
	RegisterMetaCatalog(server, hintProbeCatalog(t))

	text := callForText(t, server, "gitlab_test_hints", map[string]any{"action": "list"})
	for _, want := range []string{
		"- Use gitlab_test_hints with action 'get' to read one",
		"- Use action 'other.missing' to do something else",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(text, want) {
				t.Errorf("text = %q, want it to contain %q", text, want)
			}
		})
	}
}

// guidanceIDExample reads the example ID out of the sentence a meta tool's
// header carries about the IDs its guidance names, "so issue.list is ...".
var guidanceIDExample = regexp.MustCompile(`so ([a-z0-9_]+\.[a-z0-9_]+) is \{"action":`)

// TestMetaToolDescriptions_GuidanceIDs_AreCalledAsTheHeaderSays holds the
// sentence [toolutil.MetaToolDescriptionPrefix] puts on a meta tool with
// guidance to the catalog it describes (issue 1325). The guidance names other
// actions by canonical ID, which the meta tool's action argument refuses, and
// the sentence says how to call one: the action after the dot on the
// gitlab_<domain> meta tool, or that tool itself where it takes no action.
// Every ID any meta tool's guidance names has to be called that way, a catalog
// ID through its meta tool and a standalone ID through its own tool, and each
// sentence's example has to be an action of the tool that carries it. The
// sentence is one rule for every ID rather than a translation of each, so an
// action registered under another tool or another name would be called wrong
// by every model that read it, and this is what would notice.
func TestMetaToolDescriptions_GuidanceIDs_AreCalledAsTheHeaderSays(t *testing.T) {
	catalog := mustBuildActionCatalog(t, nil, ActionCatalogOptions{Tier: edition.Ultimate, IncludeMCP: true})
	actions := make(map[string]actioncatalog.Action)
	for _, action := range catalog.Actions() {
		actions[string(action.ID)] = action
	}
	standalone := make(map[string]string)
	for _, spec := range StandaloneSurfaceToolSpecs(nil) {
		standalone[spec.BaseDomain+"."+spec.ActionName] = spec.Name
	}

	var catalogIDs, standaloneIDs int
	for _, group := range catalog.Groups() {
		prefix := toolutil.MetaToolDescriptionPrefix(group.ToolName, group.ActionMap())
		if example := guidanceIDExample.FindStringSubmatch(prefix); example != nil {
			if action, ok := actions[example[1]]; !ok || action.ToolName != group.ToolName {
				t.Errorf("%s gives %s as its example, which is no action of that tool", group.ToolName, example[1])
			}
		}
		named, namedStandalone := checkGuidanceIDCalls(t, group.ToolName, prefix, actions, standalone)
		catalogIDs += named
		standaloneIDs += namedStandalone
	}
	if catalogIDs == 0 || standaloneIDs == 0 {
		t.Errorf("the meta descriptions name %d catalog and %d standalone IDs, want some of each for this test to hold anything",
			catalogIDs, standaloneIDs)
	}
}

// guidanceDottedWord is a dotted word of a meta description's header, which
// may be a canonical ID, an action and parameter pair, or neither.
var guidanceDottedWord = regexp.MustCompile(`[a-z0-9_]+\.[a-z0-9_]+`)

// checkGuidanceIDCalls fails the test for each canonical ID the header of the
// meta tool toolName names that is not called the way the header says, and
// returns how many catalog and standalone IDs it named.
func checkGuidanceIDCalls(t *testing.T, toolName, prefix string, actions map[string]actioncatalog.Action, standalone map[string]string) (catalogIDs, standaloneIDs int) {
	t.Helper()
	for _, id := range guidanceDottedWord.FindAllString(prefix, -1) {
		domain, name, _ := strings.Cut(id, ".")
		if action, ok := actions[id]; ok {
			catalogIDs++
			if action.ToolName != "gitlab_"+domain || action.Name != name {
				t.Errorf("%s names %s, which the meta surface calls as %s with action %q, not as gitlab_%s with action %q",
					toolName, id, action.ToolName, action.Name, domain, name)
			}
			continue
		}
		if tool, ok := standalone[id]; ok {
			standaloneIDs++
			if tool != "gitlab_"+domain {
				t.Errorf("%s names the standalone %s, whose tool is %s, not gitlab_%s", toolName, id, tool, domain)
			}
		}
	}
	return catalogIDs, standaloneIDs
}
