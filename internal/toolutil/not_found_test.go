// not_found_test.go verifies the structured 404 result builder used by
// get-handlers across all domain sub-packages when a resource is not found.
package toolutil

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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
