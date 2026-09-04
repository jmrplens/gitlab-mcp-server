// model_facing_test.go verifies that the footprint measurement counts what a
// model receives rather than what crosses the wire.
package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// sampleIcons is a stand-in for the base64 SVG data URIs the real catalog
// carries. Its marker string makes an accidental inclusion obvious in output.
var sampleIcons = []mcp.Icon{{
	Source:   "data:image/svg+xml;base64,SUNPTi1QQVlMT0FE",
	MIMEType: "image/svg+xml",
	Sizes:    []string{"any"},
}}

// TestMarshalModelFacing_OmitsIconsAndPreservesInput verifies both halves of
// the contract for every catalog type: the serialized form carries no icons,
// and the caller's entry still does. The second half matters because these
// entries come from a live session, so clearing them in place would corrupt
// any later measurement of the same list.
func TestMarshalModelFacing_OmitsIconsAndPreservesInput(t *testing.T) {
	tests := []struct {
		name  string
		entry any
		icons func(any) []mcp.Icon
	}{
		{
			name:  "tool",
			entry: &mcp.Tool{Name: "gitlab_project_get", Icons: sampleIcons},
			icons: func(e any) []mcp.Icon { return e.(*mcp.Tool).Icons },
		},
		{
			name:  "resource",
			entry: &mcp.Resource{URI: "gitlab://groups", Name: "groups", Icons: sampleIcons},
			icons: func(e any) []mcp.Icon { return e.(*mcp.Resource).Icons },
		},
		{
			name:  "resource template",
			entry: &mcp.ResourceTemplate{URITemplate: "gitlab://project/{id}", Name: "project", Icons: sampleIcons},
			icons: func(e any) []mcp.Icon { return e.(*mcp.ResourceTemplate).Icons },
		},
		{
			name:  "prompt",
			entry: &mcp.Prompt{Name: "review_mr", Icons: sampleIcons},
			icons: func(e any) []mcp.Icon { return e.(*mcp.Prompt).Icons },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := marshalModelFacing(tt.entry)
			if err != nil {
				t.Fatalf("marshalModelFacing() error: %v", err)
			}

			if strings.Contains(string(raw), "ICON-PAYLOAD") || strings.Contains(string(raw), "\"icons\"") {
				t.Errorf("serialized entry still carries icons: %s", raw)
			}
			if got := tt.icons(tt.entry); len(got) != 1 {
				t.Errorf("the caller's entry lost its icons: %+v", got)
			}
		})
	}
}

// TestMarshalModelFacing_KeepsEverythingElse verifies the narrowing removes
// only the presentation field. A measurement that dropped descriptions or
// schemas would understate the figure just as badly as counting icons
// overstated it.
func TestMarshalModelFacing_KeepsEverythingElse(t *testing.T) {
	tool := &mcp.Tool{
		Name:        "gitlab_project_get",
		Title:       "Get Project",
		Description: "Fetch one project.",
		InputSchema: map[string]any{"type": "object"},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		Icons:       sampleIcons,
	}

	raw, err := marshalModelFacing(tool)
	if err != nil {
		t.Fatalf("marshalModelFacing() error: %v", err)
	}

	var got map[string]any
	if unmarshalErr := json.Unmarshal(raw, &got); unmarshalErr != nil {
		t.Fatalf("unmarshal: %v", unmarshalErr)
	}
	for _, key := range []string{"name", "title", "description", "inputSchema", "annotations"} {
		t.Run(key, func(t *testing.T) {
			if _, ok := got[key]; !ok {
				t.Errorf("%q was dropped alongside the icons: %s", key, raw)
			}
		})
	}
}

// TestMarshalModelFacing_MarshalErrorsSurface verifies the error return, which
// nothing else exercises. A channel cannot be serialized, so it reaches the
// error path through both the typed branches and the default one.
func TestMarshalModelFacing_MarshalErrorsSurface(t *testing.T) {
	tests := []struct {
		name  string
		entry any
	}{
		{
			name:  "a tool whose input schema cannot be serialized",
			entry: &mcp.Tool{Name: "gitlab_broken", InputSchema: make(chan int)},
		},
		{
			name:  "an unknown type that cannot be serialized",
			entry: make(chan int),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := marshalModelFacing(tt.entry)
			if err == nil {
				t.Fatalf("marshalModelFacing() error = nil, want a marshal error; raw = %s", raw)
			}
			if raw != nil {
				t.Errorf("raw = %s, want nil alongside the error", raw)
			}
		})
	}
}

// TestMarshalModelFacing_UnknownTypePassesThrough verifies the default branch,
// so a result type without a presentation field still measures rather than
// silently returning nothing.
func TestMarshalModelFacing_UnknownTypePassesThrough(t *testing.T) {
	raw, err := marshalModelFacing(map[string]string{"kind": "other"})
	if err != nil {
		t.Fatalf("marshalModelFacing() error: %v", err)
	}
	if string(raw) != `{"kind":"other"}` {
		t.Errorf("raw = %s, want the entry marshaled unchanged", raw)
	}
}
