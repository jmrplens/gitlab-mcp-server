// icons_test.go validates that all icon constants are properly formed.

package toolutil

import (
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
	}
}

// TestAllIcons_ValidDataURI verifies every icon has a valid data: SVG URI prefix.
func TestAllIcons_ValidDataURI(t *testing.T) {
	const prefix = "data:image/svg+xml,"
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

// TestAllIcons_ContainsSVG verifies every icon contains SVG markup.
func TestAllIcons_ContainsSVG(t *testing.T) {
	for name, icons := range allIcons() {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(icons[0].Source, "<svg") {
				t.Error("Source does not contain <svg element")
			}
		})
	}
}
