// annotations.go provides shared MCP tool annotation presets and helpers.

package toolutil

import "github.com/modelcontextprotocol/go-sdk/mcp"

// BoolPtr returns a pointer to the given bool value.
//
//go:fix inline
func BoolPtr(b bool) *bool { return new(b) }

// StepFormattingResponse is a standard progress step label for response formatting.
const StepFormattingResponse = "Formatting response..."

// Tool annotation presets for different operation categories.
// Each preset configures MCP hints that help LLMs understand
// whether a tool is read-only, destructive, or idempotent.
var (
	ReadAnnotations = &mcp.ToolAnnotations{
		ReadOnlyHint:    true,
		DestructiveHint: new(false),
		IdempotentHint:  true,
		OpenWorldHint:   new(true),
	}
	CreateAnnotations = &mcp.ToolAnnotations{
		DestructiveHint: new(false),
		OpenWorldHint:   new(true),
	}
	UpdateAnnotations = &mcp.ToolAnnotations{
		DestructiveHint: new(false),
		IdempotentHint:  true,
		OpenWorldHint:   new(true),
	}
	DeleteAnnotations = &mcp.ToolAnnotations{
		DestructiveHint: new(true),
		IdempotentHint:  true,
		OpenWorldHint:   new(true),
	}
	// NonDestructiveMetaAnnotations are for meta-tools that include
	// create/update operations but no delete actions.
	NonDestructiveMetaAnnotations = &mcp.ToolAnnotations{
		DestructiveHint: new(false),
		OpenWorldHint:   new(true),
	}
	// MetaAnnotations are annotations for meta-tools that combine read/write/delete.
	// Since a single meta-tool may include destructive actions, annotations reflect
	// the most cautious combination.
	MetaAnnotations = &mcp.ToolAnnotations{
		DestructiveHint: new(true),
		OpenWorldHint:   new(true),
	}
	// ReadOnlyMetaAnnotations are for meta-tools with only list/get/search actions.
	ReadOnlyMetaAnnotations = &mcp.ToolAnnotations{
		ReadOnlyHint:    true,
		DestructiveHint: new(false),
		IdempotentHint:  true,
		OpenWorldHint:   new(true),
	}
)

// Content annotation presets for TextContent responses.
// These guide MCP clients on who the content is intended for and its importance.
var (
	// ContentBoth marks content for both user display and LLM processing (default).
	ContentBoth = &mcp.Annotations{
		Audience: []mcp.Role{"user", "assistant"},
		Priority: 0.5,
	}
	// ContentUser marks content primarily for user display (uploads, visualizations).
	ContentUser = &mcp.Annotations{
		Audience: []mcp.Role{"user"},
		Priority: 0.8,
	}
	// ContentAssistant marks content primarily for LLM processing (search, raw data).
	ContentAssistant = &mcp.Annotations{
		Audience: []mcp.Role{"assistant"},
		Priority: 0.7,
	}
)

// Operation-based content annotation presets.
// All use audience ["assistant"] so the Markdown content is available to the LLM
// for reasoning, while StructuredContent (JSON) serves programmatic clients.
// This avoids redundant display when clients show both Content and StructuredContent.
var (
	// ContentList marks list/search results for LLM processing with lower priority.
	ContentList = &mcp.Annotations{
		Audience: []mcp.Role{"assistant"},
		Priority: 0.4,
	}
	// ContentDetail marks single-entity details for LLM processing with medium priority.
	ContentDetail = &mcp.Annotations{
		Audience: []mcp.Role{"assistant"},
		Priority: 0.6,
	}
	// ContentMutate marks create/update/delete results for LLM processing with high priority.
	ContentMutate = &mcp.Annotations{
		Audience: []mcp.Role{"assistant"},
		Priority: 0.8,
	}
)
