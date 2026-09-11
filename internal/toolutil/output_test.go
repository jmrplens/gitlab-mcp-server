// output_test.go contains unit tests for MCP tool output helpers.
package toolutil

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestSuccessResult verifies that [SuccessResult] returns a non-nil result
// with IsError=false for non-empty markdown, and nil for empty input.
func TestSuccessResult(t *testing.T) {
	t.Run("non-empty markdown", func(t *testing.T) {
		result := SuccessResult("## Title\nSome content")
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if len(result.Content) != 1 {
			t.Fatalf("expected 1 content item, got %d", len(result.Content))
		}
		if result.IsError {
			t.Error("expected IsError=false")
		}
	})

	t.Run("empty markdown returns nil", func(t *testing.T) {
		result := SuccessResult("")
		if result != nil {
			t.Error("expected nil for empty markdown")
		}
	})
}

// TestErrorResultAnnotated_Cases_BuildsTheOneRefusalEnvelope verifies the
// envelope every refusal travels in: IsError set, one text block normalized
// like a success (trailing whitespace and control bytes dropped), and the
// annotation given, or the refusal preset for nil, so no error block leaves
// without one.
func TestErrorResultAnnotated_Cases_BuildsTheOneRefusalEnvelope(t *testing.T) {
	cases := []struct {
		name    string
		md      string
		ann     *mcp.Annotations
		wantMd  string
		wantAnn *mcp.Annotations
	}{
		{name: "detail preset", md: "## Not Found\n", ann: ContentDetail, wantMd: "## Not Found\n", wantAnn: ContentDetail},
		{name: "nil is the refusal preset", md: "refused", ann: nil, wantMd: "refused", wantAnn: ContentMutate},
		{name: "normalized", md: "line\x1b[2J  \nnext\t\n", ann: ContentMutate, wantMd: "line[2J\nnext\n", wantAnn: ContentMutate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := ErrorResultAnnotated(tc.md, tc.ann)
			if result == nil || !result.IsError || len(result.Content) != 1 {
				t.Fatalf("ErrorResultAnnotated() = %+v, want one error block", result)
			}
			text, ok := result.Content[0].(*mcp.TextContent)
			if !ok {
				t.Fatalf("block is %T, want text", result.Content[0])
			}
			if text.Text != tc.wantMd {
				t.Errorf("text = %q, want %q", text.Text, tc.wantMd)
			}
			if text.Annotations != tc.wantAnn {
				t.Errorf("annotations = %+v, want %+v", text.Annotations, tc.wantAnn)
			}
		})
	}
}

// TestErrorResult verifies that [ErrorResult] returns a non-nil result
// with IsError=true and a single content entry annotated as a refusal.
func TestErrorResult(t *testing.T) {
	result := ErrorResult("something went wrong")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
	text := result.Content[0].(*mcp.TextContent)
	if text.Text != "something went wrong" || text.Annotations != ContentMutate {
		t.Errorf("block = %+v, want the message annotated as a refusal", text)
	}
}
