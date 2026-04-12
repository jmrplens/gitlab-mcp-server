// hints_test.go contains unit tests for WriteHints and ExtractHints helpers.
package toolutil

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestWriteHints_MultipleHints verifies that WriteHints appends multiple hint
// lines as a formatted next-steps section to a strings.Builder.
func TestWriteHints_MultipleHints(t *testing.T) {
	var b strings.Builder
	b.WriteString("## Title\n")
	WriteHints(&b, "Use 'delete' to remove this item", "Use 'list' to see all items")

	got := b.String()
	if !strings.Contains(got, "💡 **Next steps:**") {
		t.Error("expected next steps header")
	}
	if !strings.Contains(got, "- Use 'delete' to remove this item") {
		t.Error("expected first hint")
	}
	if !strings.Contains(got, "- Use 'list' to see all items") {
		t.Error("expected second hint")
	}
}

// TestWriteHints_NoHints verifies that WriteHints writes nothing when called
// with no hint arguments.
func TestWriteHints_NoHints(t *testing.T) {
	var b strings.Builder
	b.WriteString("## Title\n")
	WriteHints(&b)

	got := b.String()
	if strings.Contains(got, "Next steps") {
		t.Error("expected no hints section when no hints provided")
	}
	if got != "## Title\n" {
		t.Errorf("expected unchanged content, got %q", got)
	}
}

// TestWriteHints_SingleHint verifies that WriteHints correctly formats a
// single hint line in the next-steps section.
func TestWriteHints_SingleHint(t *testing.T) {
	var b strings.Builder
	WriteHints(&b, "Use 'get' to view details")

	got := b.String()
	if !strings.Contains(got, "- Use 'get' to view details") {
		t.Error("expected single hint")
	}
}

// TestExtractHints_WithHints verifies that ExtractHints extracts hint lines
// from a markdown string containing a next-steps section.
func TestExtractHints_WithHints(t *testing.T) {
	md := "## Title\n\nSome content\n\n---\n💡 **Next steps:**\n- Use action 'get' to see details\n- Use action 'delete' to remove\n"
	hints := ExtractHints(md)
	if len(hints) != 2 {
		t.Fatalf("expected 2 hints, got %d", len(hints))
	}
	if hints[0] != "Use action 'get' to see details" {
		t.Errorf("hints[0] = %q", hints[0])
	}
	if hints[1] != "Use action 'delete' to remove" {
		t.Errorf("hints[1] = %q", hints[1])
	}
}

// TestExtractHints_NoSection verifies that ExtractHints returns nil when the
// input contains no next-steps section.
func TestExtractHints_NoSection(t *testing.T) {
	md := "## Title\n\nJust some content.\n"
	if hints := ExtractHints(md); hints != nil {
		t.Errorf("expected nil, got %v", hints)
	}
}

// TestExtractHints_EmptyString verifies that ExtractHints returns nil for an
// empty input string.
func TestExtractHints_EmptyString(t *testing.T) {
	if hints := ExtractHints(""); hints != nil {
		t.Errorf("expected nil, got %v", hints)
	}
}

// TestExtractHints_RoundTrip verifies that hints written by WriteHints can
// be extracted back by ExtractHints, forming a round-trip.
func TestExtractHints_RoundTrip(t *testing.T) {
	var b strings.Builder
	b.WriteString("## Results\n\n| Col |\n| --- |\n| val |\n")
	WriteHints(&b, "First hint", "Second hint")
	md := b.String()

	hints := ExtractHints(md)
	if len(hints) != 2 {
		t.Fatalf("expected 2 hints, got %d", len(hints))
	}
	if hints[0] != "First hint" {
		t.Errorf("hints[0] = %q, want %q", hints[0], "First hint")
	}
	if hints[1] != "Second hint" {
		t.Errorf("hints[1] = %q, want %q", hints[1], "Second hint")
	}
}

// hintTestOutput is a sample struct embedding HintableOutput for testing.
type hintTestOutput struct {
	HintableOutput
	Name  string `json:"name"`
	Value int    `json:"value"`
}

// TestPopulateHints_WithHints verifies that PopulateHints extracts hints from
// TextContent and sets them on the output struct.
func TestPopulateHints_WithHints(t *testing.T) {
	var b strings.Builder
	b.WriteString("## Branch\n| Name |\n| --- |\n| main |\n")
	WriteHints(&b, "Use 'delete' to remove", "Use 'list' to see all")

	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: b.String()},
		},
	}
	out := &hintTestOutput{Name: "main", Value: 42}
	PopulateHints(result, out)

	if len(out.NextSteps) != 2 {
		t.Fatalf("expected 2 hints, got %d", len(out.NextSteps))
	}
	if out.NextSteps[0] != "Use 'delete' to remove" {
		t.Errorf("NextSteps[0] = %q", out.NextSteps[0])
	}
	if out.NextSteps[1] != "Use 'list' to see all" {
		t.Errorf("NextSteps[1] = %q", out.NextSteps[1])
	}
}

// TestPopulateHints_NoHints verifies that PopulateHints leaves NextSteps nil
// when the result has no hints section.
func TestPopulateHints_NoHints(t *testing.T) {
	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "## Just content\nNo hints here."},
		},
	}
	out := &hintTestOutput{Name: "test"}
	PopulateHints(result, out)

	if out.NextSteps != nil {
		t.Errorf("expected nil NextSteps, got %v", out.NextSteps)
	}
}

// TestPopulateHints_NilResult verifies that PopulateHints is a no-op
// when result is nil.
func TestPopulateHints_NilResult(t *testing.T) {
	out := &hintTestOutput{Name: "test"}
	PopulateHints(nil, out)

	if out.NextSteps != nil {
		t.Errorf("expected nil NextSteps, got %v", out.NextSteps)
	}
}

// TestPopulateHints_NilSetter verifies that PopulateHints is a no-op
// when setter is nil.
func TestPopulateHints_NilSetter(t *testing.T) {
	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "## Content\n---\n💡 **Next steps:**\n- hint\n"},
		},
	}
	PopulateHints(result, nil) // should not panic
}

// TestPopulateHints_NoTextContent verifies that PopulateHints is a no-op
// when result has no TextContent items.
func TestPopulateHints_NoTextContent(t *testing.T) {
	result := &mcp.CallToolResult{
		Content: []mcp.Content{},
	}
	out := &hintTestOutput{Name: "test"}
	PopulateHints(result, out)

	if out.NextSteps != nil {
		t.Errorf("expected nil NextSteps, got %v", out.NextSteps)
	}
}

// TestHintableOutput_JSONWithHints verifies that next_steps appears as the
// first field in JSON when hints are populated.
func TestHintableOutput_JSONWithHints(t *testing.T) {
	out := hintTestOutput{
		HintableOutput: HintableOutput{NextSteps: []string{"hint1", "hint2"}},
		Name:           "test",
		Value:          1,
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	s := string(data)
	if !strings.HasPrefix(s, `{"next_steps":`) {
		t.Errorf("expected next_steps as first field, got: %s", s)
	}
	if !strings.Contains(s, `"name":"test"`) {
		t.Errorf("expected name field, got: %s", s)
	}
}

// TestHintableOutput_JSONWithoutHints verifies that next_steps is absent from
// JSON when no hints are set (omitempty).
func TestHintableOutput_JSONWithoutHints(t *testing.T) {
	out := hintTestOutput{Name: "test", Value: 1}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	s := string(data)
	if strings.Contains(s, "next_steps") {
		t.Errorf("expected no next_steps field, got: %s", s)
	}
	if !strings.HasPrefix(s, `{"name":`) {
		t.Errorf("expected name as first field when no hints, got: %s", s)
	}
}

// TestDeleteOutput_EmbedHintableOutput verifies that DeleteOutput includes
// the HintableOutput embed and serializes correctly.
func TestDeleteOutput_EmbedHintableOutput(t *testing.T) {
	out := DeleteOutput{Status: "success", Message: "deleted"}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	s := string(data)
	if strings.Contains(s, "next_steps") {
		t.Errorf("expected no next_steps when empty, got: %s", s)
	}

	out.SetNextSteps([]string{"use list"})
	data, err = json.Marshal(out)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	s = string(data)
	if !strings.HasPrefix(s, `{"next_steps":`) {
		t.Errorf("expected next_steps first, got: %s", s)
	}
}

// TestVoidOutput_EmbedHintableOutput verifies that VoidOutput includes
// the HintableOutput embed and serializes correctly.
func TestVoidOutput_EmbedHintableOutput(t *testing.T) {
	out := VoidOutput{Status: "success", Message: "done"}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if strings.Contains(string(data), "next_steps") {
		t.Errorf("expected no next_steps when empty, got: %s", data)
	}

	out.SetNextSteps([]string{"check status"})
	data, err = json.Marshal(out)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !strings.HasPrefix(string(data), `{"next_steps":`) {
		t.Errorf("expected next_steps first, got: %s", data)
	}
}

// TestWithHints_ValueOutType verifies that WithHints populates hints on a
// value Out type (the common handler pattern) and returns all three values.
func TestWithHints_ValueOutType(t *testing.T) {
	var b strings.Builder
	b.WriteString("## Result\n")
	WriteHints(&b, "hint1", "hint2")

	result := &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: b.String()}},
	}
	out := hintTestOutput{Name: "test", Value: 1}

	gotResult, gotOut, gotErr := WithHints(result, out, nil)
	if gotErr != nil {
		t.Fatalf("unexpected error: %v", gotErr)
	}
	if gotResult != result {
		t.Error("expected same result pointer")
	}
	if len(gotOut.NextSteps) != 2 {
		t.Fatalf("expected 2 hints, got %d", len(gotOut.NextSteps))
	}
	if gotOut.NextSteps[0] != "hint1" {
		t.Errorf("NextSteps[0] = %q", gotOut.NextSteps[0])
	}
}

// TestWithHints_PointerOutType verifies that WithHints works with pointer
// Out types where the pointer itself implements HintSetter.
func TestWithHints_PointerOutType(t *testing.T) {
	var b strings.Builder
	b.WriteString("## Result\n")
	WriteHints(&b, "ptr-hint")

	result := &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: b.String()}},
	}
	out := &hintTestOutput{Name: "ptr"}

	gotResult, gotOut, gotErr := WithHints(result, out, nil)
	if gotErr != nil {
		t.Fatalf("unexpected error: %v", gotErr)
	}
	if gotResult != result {
		t.Error("expected same result pointer")
	}
	if len(gotOut.NextSteps) != 1 || gotOut.NextSteps[0] != "ptr-hint" {
		t.Errorf("expected [ptr-hint], got %v", gotOut.NextSteps)
	}
}

// TestWithHints_ErrorSkipsHints verifies that WithHints skips hint
// population when err is non-nil.
func TestWithHints_ErrorSkipsHints(t *testing.T) {
	var b strings.Builder
	b.WriteString("## Result\n")
	WriteHints(&b, "should-not-appear")

	result := &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: b.String()}},
	}
	out := hintTestOutput{Name: "test"}

	_, gotOut, gotErr := WithHints(result, out, fmt.Errorf("fail"))
	if gotErr == nil {
		t.Fatal("expected error")
	}
	if gotOut.NextSteps != nil {
		t.Errorf("expected nil NextSteps on error, got %v", gotOut.NextSteps)
	}
}

// TestWithHints_NoHintSetter verifies that WithHints is a no-op for types
// that do not embed HintableOutput.
func TestWithHints_NoHintSetter(t *testing.T) {
	type plainOutput struct {
		Name string `json:"name"`
	}
	result := &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "## x\n---\n💡 **Next steps:**\n- hint\n"}},
	}
	out := plainOutput{Name: "test"}

	gotResult, gotOut, gotErr := WithHints(result, out, nil)
	if gotErr != nil {
		t.Fatalf("unexpected error: %v", gotErr)
	}
	if gotResult != result {
		t.Error("expected same result pointer")
	}
	if gotOut.Name != "test" {
		t.Errorf("expected name=test, got %q", gotOut.Name)
	}
}
