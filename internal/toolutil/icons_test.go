// icons_test.go validates that all icon constants are properly formed.

package toolutil

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// allIcons returns every domain icon slice for exhaustive validation.
func allIcons() map[string][]mcp.Icon {
	return map[string][]mcp.Icon{
		"Branch":      IconBranch,
		"Commit":      IconCommit,
		"Issue":       IconIssue,
		"MR":          IconMR,
		"Pipeline":    IconPipeline,
		"Job":         IconJob,
		"Release":     IconRelease,
		"Tag":         IconTag,
		"Project":     IconProject,
		"Group":       IconGroup,
		"User":        IconUser,
		"Wiki":        IconWiki,
		"File":        IconFile,
		"Package":     IconPackage,
		"Search":      IconSearch,
		"Label":       IconLabel,
		"Milestone":   IconMilestone,
		"Environment": IconEnvironment,
		"Deploy":      IconDeploy,
		"Schedule":    IconSchedule,
		"Variable":    IconVariable,
		"Runner":      IconRunner,
		"Todo":        IconTodo,
		"Health":      IconHealth,
		"Upload":      IconUpload,
		"Board":       IconBoard,
		"Snippet":     IconSnippet,
		"Token":       IconToken,
		"Integration": IconIntegration,
		"Notify":      IconNotify,
		"Server":      IconServer,
		"Security":    IconSecurity,
		"Config":      IconConfig,
		"Analytics":   IconAnalytics,
		"Key":         IconKey,
		"Link":        IconLink,
		"Discussion":  IconDiscussion,
		"Event":       IconEvent,
		"Container":   IconContainer,
		"Import":      IconImport,
		"Alert":       IconAlert,
		"Template":    IconTemplate,
		"Infra":       IconInfra,
		"Epic":        IconEpic,
	}
}

// TestAllIcons_ValidDataURI verifies every icon uses a valid base64-encoded
// data URI prefix per RFC 2397.
func TestAllIcons_ValidDataURI(t *testing.T) {
	const prefix = "data:image/svg+xml;base64,"
	for name, icons := range allIcons() {
		t.Run(name, func(t *testing.T) {
			if len(icons) != 1 {
				t.Fatalf("expected 1 icon, got %d", len(icons))
			}
			ic := icons[0]
			if !strings.HasPrefix(ic.Source, prefix) {
				t.Errorf("Source does not start with %q: %s", prefix, ic.Source[:min(60, len(ic.Source))])
			}
		})
	}
}

// TestAllIcons_CorrectMIMEType verifies every icon reports the correct MIME type.
func TestAllIcons_CorrectMIMEType(t *testing.T) {
	for name, icons := range allIcons() {
		t.Run(name, func(t *testing.T) {
			if icons[0].MIMEType != "image/svg+xml" {
				t.Errorf("MIMEType = %q, want %q", icons[0].MIMEType, "image/svg+xml")
			}
		})
	}
}

// TestAllIcons_NonEmpty verifies no icon has an empty Source.
func TestAllIcons_NonEmpty(t *testing.T) {
	for name, icons := range allIcons() {
		t.Run(name, func(t *testing.T) {
			if icons[0].Source == "" {
				t.Error("Source is empty")
			}
		})
	}
}

// TestAllIcons_DecodesToSVG verifies the base64-encoded payload decodes to
// well-formed SVG markup. This catches regressions where the encoder emits
// invalid base64 or where a raw SVG sneaks back into the data URI.
func TestAllIcons_DecodesToSVG(t *testing.T) {
	const prefix = "data:image/svg+xml;base64,"
	for name, icons := range allIcons() {
		t.Run(name, func(t *testing.T) {
			payload := strings.TrimPrefix(icons[0].Source, prefix)
			decoded, err := base64.StdEncoding.DecodeString(payload)
			if err != nil {
				t.Fatalf("base64 decode failed: %v", err)
			}
			if !strings.HasPrefix(string(decoded), "<svg") {
				t.Errorf("decoded payload does not start with <svg: %q", string(decoded[:min(40, len(decoded))]))
			}
			if !strings.HasSuffix(string(decoded), "</svg>") {
				t.Errorf("decoded payload does not end with </svg>")
			}
		})
	}
}

// TestAllIcons_SizesAny verifies every icon advertises Sizes=["any"] so
// clients know the SVG is resolution-independent.
func TestAllIcons_SizesAny(t *testing.T) {
	for name, icons := range allIcons() {
		t.Run(name, func(t *testing.T) {
			sizes := icons[0].Sizes
			if len(sizes) != 1 || sizes[0] != "any" {
				t.Errorf("Sizes = %v, want [\"any\"]", sizes)
			}
		})
	}
}
