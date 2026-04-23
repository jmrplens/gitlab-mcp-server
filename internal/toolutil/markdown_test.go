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

// TestToolResultWithImage verifies that ToolResultWithImage creates a
// CallToolResult containing both a TextContent with metadata and an
// ImageContent with raw image bytes and MIME type. Covers valid inputs,
// nil annotations, and empty image data to ensure all branches produce
// the expected two-element Content slice.
func TestToolResultWithImage_Scenarios_CorrectContent(t *testing.T) {
	tests := []struct {
		name      string
		md        string
		ann       *mcp.Annotations
		imageData []byte
		mimeType  string
		wantText  string
		wantMIME  string
		wantAnn   bool
	}{
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
			result := ToolResultWithImage(tt.md, tt.ann, tt.imageData, tt.mimeType)
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if len(result.Content) != 2 {
				t.Fatalf("expected 2 content items, got %d", len(result.Content))
			}

			tc, ok := result.Content[0].(*mcp.TextContent)
			if !ok {
				t.Fatal("first content item should be TextContent")
			}
			if tc.Text != tt.wantText {
				t.Errorf("TextContent.Text = %q, want %q", tc.Text, tt.wantText)
			}
			if tt.wantAnn && tc.Annotations == nil {
				t.Error("expected annotations to be set")
			}
			if !tt.wantAnn && tc.Annotations != nil {
				t.Errorf("expected nil annotations, got %v", tc.Annotations)
			}

			ic, ok := result.Content[1].(*mcp.ImageContent)
			if !ok {
				t.Fatal("second content item should be ImageContent")
			}
			if ic.MIMEType != tt.wantMIME {
				t.Errorf("ImageContent.MIMEType = %q, want %q", ic.MIMEType, tt.wantMIME)
			}
			if !bytes.Equal(ic.Data, tt.imageData) {
				t.Errorf("ImageContent.Data mismatch: got %v, want %v", ic.Data, tt.imageData)
			}
		})
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
