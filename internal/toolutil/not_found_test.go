// not_found_test.go verifies the structured 404 result builder used by
// get-handlers across all domain sub-packages when a resource is not found.
package toolutil

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// TestNotFoundResult verifies the not-found card byte for byte: the heading
// naming the resource, the sentence naming the identifier the caller passed,
// the rule WriteHints emits and the domain hints, in an error result
// annotated as a detail.
func TestNotFoundResult(t *testing.T) {
	result := NotFoundResult("Project", "42", "Use gitlab_project_list to search", "Check permissions")
	if result == nil || !result.IsError || len(result.Content) != 1 {
		t.Fatalf("NotFoundResult() = %+v, want one error block", result)
	}
	text := result.Content[0].(*mcp.TextContent)
	want := "## " + EmojiQuestion + " Project Not Found\n\n" +
		"The project **42** does not exist or is not accessible with your current permissions.\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use gitlab_project_list to search\n" +
		"- Check permissions\n"
	if text.Text != want {
		t.Errorf("not-found card:\n got %q\nwant %q", text.Text, want)
	}
	if text.Annotations != ContentDetail {
		t.Errorf("annotations = %+v, want the detail preset", text.Annotations)
	}
	if hints := ExtractHints(text.Text); len(hints) != 2 {
		t.Errorf("ExtractHints() = %q, want the two domain hints", hints)
	}
}

// TestNotFoundResult_HostileResourceLabel_ReachesBothSlotsAsText verifies the
// other half of the sentence, the resource label, byte for byte in both places
// it lands: the heading, through the card writer's heading escaper, and the
// sentence, lowered and then escaped for the inline slot it sits in.
//
// The label used to be interpolated raw, on the strength of a doc comment
// saying every caller passes a constant. Two callers did not: badges reads it
// off its result and escaped it at its own call site, and orbit read it off
// its result and passed it through. A rule kept in a comment is a rule half
// the callers follow, so it is kept here instead, and this test is what says
// so.
func TestNotFoundResult_HostileResourceLabel_ReachesBothSlotsAsText(t *testing.T) {
	result := NotFoundResult("Award <b>Emoji", "7")
	if result == nil || !result.IsError {
		t.Fatal("expected an error result")
	}
	text := result.Content[0].(*mcp.TextContent).Text
	want := "## " + EmojiQuestion + " Award &lt;b>Emoji Not Found\n\n" +
		"The award &lt;b>emoji **7** does not exist or is not accessible with your current permissions.\n"
	if text != want {
		t.Errorf("not-found card:\n got %q\nwant %q", text, want)
	}
}

// TestNotFoundResult_NoHints_EscapesTheIdentifier verifies a result without
// hints ends after the sentence, and that an identifier the caller typed
// cannot open a link or a tag in it: it is escaped on its way in, with the
// bracket and the angle bracket written as entities.
func TestNotFoundResult_NoHints_EscapesTheIdentifier(t *testing.T) {
	result := NotFoundResult("Branch", "[main](http://attacker.invalid/)<b>")
	if result == nil || !result.IsError {
		t.Fatal("expected an error result")
	}
	text := result.Content[0].(*mcp.TextContent).Text
	want := "## " + EmojiQuestion + " Branch Not Found\n\n" +
		"The branch **&#91;main](http://attacker.invalid/)&lt;b>** does not exist or is not accessible with your current permissions.\n"
	if text != want {
		t.Errorf("not-found card:\n got %q\nwant %q", text, want)
	}
}

// TestParamText_NamesAnArgumentAsTheCallerWroteIt verifies the identifier a
// not-found result names is the argument as the caller wrote it: a JSON
// number, which a route wrapper reads as a float64, is printed whole at any
// size, where %v printed one of a million or more as 3.1234567e+07, while a
// string, a fraction and a missing argument read as fmt prints them.
func TestParamText_NamesAnArgumentAsTheCallerWroteIt(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value any
		want  string
	}{
		{name: "a JSON number of eight digits", value: float64(31234567), want: "31234567"},
		{name: "a small JSON number", value: float64(42), want: "42"},
		{name: "an int", value: 12345678, want: "12345678"},
		{name: "a path", value: "group/project", want: "group/project"},
		{name: "a fraction", value: 1.5, want: "1.5"},
		{name: "nothing", value: nil, want: "<nil>"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParamText(tt.value); got != tt.want {
				t.Errorf("ParamText(%#v) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

// notFoundTestInput is a get route's input with one scope and one internal
// id, the pair the two aliases the tests below send resolve to: project_path
// to project_id, and iid to the one _iid field.
type notFoundTestInput struct {
	ProjectID    string `json:"project_id"`
	MilestoneIID int64  `json:"milestone_iid"`
}

// TestActionRouteWrapNotFound_ReadsTheArgumentsTheHandlerRead verifies the
// builder of a not-found output is handed the arguments with the documented
// aliases resolved, as UnmarshalParams resolves them for the handler: the meta
// surface passes a route the caller's own spelling, and a builder reading that
// map named a project given as project_path as "<nil>". It also verifies the
// caller's map is left as it arrived, and that the reserved confirm key does
// not reach the builder.
func TestActionRouteWrapNotFound_ReadsTheArgumentsTheHandlerRead(t *testing.T) {
	t.Parallel()

	var handled notFoundTestInput
	get := func(_ context.Context, _ *gitlabclient.Client, input notFoundTestInput) (string, error) {
		handled = input
		return "", fmt.Errorf("milestoneGet: %w", gl.ErrNotFound)
	}
	var built map[string]any
	route := RouteAction(&gitlabclient.Client{}, get).WrapNotFound(func(params map[string]any) any {
		built = params
		return fmt.Sprintf("IID %s in project %s", ParamText(params["milestone_iid"]), ParamText(params["project_id"]))
	})

	input := map[string]any{"project_path": "group/project", "iid": float64(31234567), "confirm": true}
	sent := maps.Clone(input)
	result, err := route.Handler(context.Background(), input)
	if err != nil || result != "IID 31234567 in project group/project" {
		t.Fatalf("Handler() = %v, %v; want the not-found output naming both arguments", result, err)
	}
	if handled != (notFoundTestInput{ProjectID: "group/project", MilestoneIID: 31234567}) {
		t.Errorf("handler input = %+v, want both aliases resolved", handled)
	}
	want := map[string]any{"project_id": "group/project", "milestone_iid": float64(31234567)}
	if !maps.Equal(built, want) {
		t.Errorf("builder params = %v, want %v", built, want)
	}
	if !maps.Equal(input, sent) {
		t.Errorf("caller's arguments = %v, want them as sent, %v", input, sent)
	}
}

// TestActionRouteWrapNotFound_PassesEveryOtherAnswerThrough verifies only a
// 404 is replaced: a result, and an error with any other status, reach the
// caller as the handler returned them, and the builder is never called.
func TestActionRouteWrapNotFound_PassesEveryOtherAnswerThrough(t *testing.T) {
	t.Parallel()

	refused := &gl.ErrorResponse{Response: &http.Response{StatusCode: http.StatusForbidden}}
	for _, tt := range []struct {
		name       string
		result     string
		err        error
		wantResult any
	}{
		{name: "a result", result: "found", wantResult: "found"},
		{name: "another status", err: refused, wantResult: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			get := func(context.Context, *gitlabclient.Client, notFoundTestInput) (string, error) {
				return tt.result, tt.err
			}
			route := RouteAction(&gitlabclient.Client{}, get).WrapNotFound(func(map[string]any) any {
				t.Error("builder called for an answer that was not a 404")
				return nil
			})
			result, err := route.Handler(context.Background(), map[string]any{"project_id": "42"})
			if result != tt.wantResult || !errors.Is(err, tt.err) {
				t.Errorf("Handler() = %v, %v; want %v, %v", result, err, tt.wantResult, tt.err)
			}
		})
	}
}

// TestActionRouteWrapNotFound_KeepsTheDecorationAcrossRebinding verifies the
// not-found answer survives BindTo, which is how a shared catalog serves a
// route to another credential, and that a route with no input type hands its
// builder the arguments as they arrived, having no field names to resolve an
// alias against.
func TestActionRouteWrapNotFound_KeepsTheDecorationAcrossRebinding(t *testing.T) {
	t.Parallel()

	get := func(context.Context, *gitlabclient.Client, notFoundTestInput) (string, error) {
		return "", gl.ErrNotFound
	}
	named := func(params map[string]any) any { return ParamText(params["project_id"]) }
	rebound := RouteAction(&gitlabclient.Client{}, get).WrapNotFound(named).BindTo(&gitlabclient.Client{})
	if result, err := rebound.Handler(context.Background(), map[string]any{"project_path": "group/project"}); err != nil || result != "group/project" {
		t.Fatalf("rebound Handler() = %v, %v; want the not-found output", result, err)
	}
	if err := ValidateRouteBinding(rebound); err != nil {
		t.Fatalf("ValidateRouteBinding(rebound) = %v, want nil", err)
	}

	untyped := Route(func(context.Context, map[string]any) (any, error) { return nil, gl.ErrNotFound }).
		WrapNotFound(func(params map[string]any) any { return ParamText(params["project_path"]) })
	if result, err := untyped.Handler(context.Background(), map[string]any{"project_path": "group/project"}); err != nil || result != "group/project" {
		t.Fatalf("untyped Handler() = %v, %v; want the argument as it arrived", result, err)
	}
}
