// icons_test.go validates that all icon constants are properly formed.
package toolutil

import (
	"encoding/base64"
	"image"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	_ "golang.org/x/image/webp" // registers the "webp" format with image.DecodeConfig
)

// allIcons returns every icon slice (domain icons plus the brand mark) for
// exhaustive validation.
func allIcons() map[string][]mcp.Icon {
	return map[string][]mcp.Icon{
		"Branch":        IconBranch,
		"Commit":        IconCommit,
		"Issue":         IconIssue,
		"MR":            IconMR,
		"Pipeline":      IconPipeline,
		"Job":           IconJob,
		"Release":       IconRelease,
		"Tag":           IconTag,
		"Project":       IconProject,
		"Group":         IconGroup,
		"User":          IconUser,
		"Wiki":          IconWiki,
		"File":          IconFile,
		"Package":       IconPackage,
		"Search":        IconSearch,
		"Label":         IconLabel,
		"Milestone":     IconMilestone,
		"Environment":   IconEnvironment,
		"Deploy":        IconDeploy,
		"Schedule":      IconSchedule,
		"Variable":      IconVariable,
		"Runner":        IconRunner,
		"Todo":          IconTodo,
		"Health":        IconHealth,
		"Upload":        IconUpload,
		"Board":         IconBoard,
		"Snippet":       IconSnippet,
		"Token":         IconToken,
		"Integration":   IconIntegration,
		"Notify":        IconNotify,
		"Server":        IconServer,
		"Security":      IconSecurity,
		"Config":        IconConfig,
		"Analytics":     IconAnalytics,
		"Key":           IconKey,
		"Link":          IconLink,
		"Discussion":    IconDiscussion,
		"Event":         IconEvent,
		"Container":     IconContainer,
		"Import":        IconImport,
		"Alert":         IconAlert,
		"Template":      IconTemplate,
		"Infra":         IconInfra,
		"Epic":          IconEpic,
		"Shield":        IconShield,
		"Audit":         IconAudit,
		"Queue":         IconQueue,
		"Bot":           IconBot,
		"Vulnerability": IconVulnerability,
		"Compliance":    IconCompliance,
		"Brand":         IconBrand,
	}
}

// TestAllIcons_ThreeEntries verifies every icon carries exactly three
// entries: the SVG plus its light and dark WebP fallbacks.
func TestAllIcons_ThreeEntries(t *testing.T) {
	for name, icons := range allIcons() {
		t.Run(name, func(t *testing.T) {
			if len(icons) != 3 {
				t.Fatalf("expected 3 icon entries (SVG + WebP light + WebP dark), got %d", len(icons))
			}
		})
	}
}

// TestAllIcons_ValidDataURI verifies every icon uses a valid base64-encoded
// data URI prefix per RFC 2397, matching its declared MIME type.
func TestAllIcons_ValidDataURI(t *testing.T) {
	for name, icons := range allIcons() {
		t.Run(name, func(t *testing.T) {
			for i, ic := range icons {
				prefix := "data:" + ic.MIMEType + ";base64,"
				if !strings.HasPrefix(ic.Source, prefix) {
					t.Errorf("entry %d: Source does not start with %q: %s", i, prefix, ic.Source[:min(60, len(ic.Source))])
				}
			}
		})
	}
}

// TestAllIcons_CorrectMIMEType verifies the SVG entry and both WebP
// fallback entries report the correct MIME type.
func TestAllIcons_CorrectMIMEType(t *testing.T) {
	for name, icons := range allIcons() {
		t.Run(name, func(t *testing.T) {
			svgIcon, light, dark := icons[0], icons[1], icons[2]
			if svgIcon.MIMEType != "image/svg+xml" {
				t.Errorf("SVG entry MIMEType = %q, want %q", svgIcon.MIMEType, "image/svg+xml")
			}
			if light.MIMEType != "image/webp" {
				t.Errorf("light entry MIMEType = %q, want %q", light.MIMEType, "image/webp")
			}
			if dark.MIMEType != "image/webp" {
				t.Errorf("dark entry MIMEType = %q, want %q", dark.MIMEType, "image/webp")
			}
		})
	}
}

// TestAllIcons_NonEmpty verifies no icon entry has an empty Source.
func TestAllIcons_NonEmpty(t *testing.T) {
	for name, icons := range allIcons() {
		t.Run(name, func(t *testing.T) {
			for i, ic := range icons {
				if ic.Source == "" {
					t.Errorf("entry %d: Source is empty", i)
				}
			}
		})
	}
}

// TestAllIcons_DecodesToSVG verifies the SVG entry's base64-encoded payload
// decodes to well-formed SVG markup. This catches regressions where the
// encoder emits invalid base64 or where a raw SVG sneaks back into the data
// URI.
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

// TestAllIcons_SizesAny verifies the SVG entry advertises Sizes=["any"] so
// clients know it is resolution-independent.
func TestAllIcons_SizesAny(t *testing.T) {
	for name, icons := range allIcons() {
		t.Run(name, func(t *testing.T) {
			sizes := icons[0].Sizes
			if len(sizes) != 1 || sizes[0] != "any" {
				t.Errorf("SVG entry Sizes = %v, want [\"any\"]", sizes)
			}
		})
	}
}

// TestAllIcons_WebPFallbackTheme verifies the light/dark WebP entries
// declare the matching mcp.IconTheme and a concrete 16x16 size, since a
// raster image is not resolution-independent like the SVG entry.
func TestAllIcons_WebPFallbackTheme(t *testing.T) {
	for name, icons := range allIcons() {
		t.Run(name, func(t *testing.T) {
			for i, entry := range []struct {
				name  string
				icon  mcp.Icon
				theme mcp.IconTheme
			}{
				{"light", icons[1], mcp.IconThemeLight},
				{"dark", icons[2], mcp.IconThemeDark},
			} {
				t.Run(entry.name, func(t *testing.T) {
					if entry.icon.Theme != entry.theme {
						t.Errorf("%s entry Theme = %q, want %q", entry.name, entry.icon.Theme, entry.theme)
					}
					if len(entry.icon.Sizes) != 1 || entry.icon.Sizes[0] != "16x16" {
						t.Errorf("WebP entry %d Sizes = %v, want [\"16x16\"]", i+1, entry.icon.Sizes)
					}
				})
			}
		})
	}
}

// TestWebpIcon_PanicsOnMissingAsset verifies webpIcon fails fast, with an
// actionable message, when an icon name has no matching embedded WebP asset
// instead of silently returning a broken or empty mcp.Icon.
func TestWebpIcon_PanicsOnMissingAsset(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("webpIcon() did not panic for a name with no embedded asset")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "missing embedded icon asset") || !strings.Contains(msg, "gen_icon_webp") {
			t.Errorf("panic value = %v, want it to name the missing asset and point at cmd/gen_icon_webp", r)
		}
	}()
	webpIcon("this-icon-name-has-no-webp-asset", "light", mcp.IconThemeLight)
}

// TestAllIcons_WebPFallbackDecodes verifies the light/dark WebP payloads
// decode to a valid 16x16 image, catching a corrupt embed or a stale
// generated asset that no longer matches its declared Sizes.
func TestAllIcons_WebPFallbackDecodes(t *testing.T) {
	const prefix = "data:image/webp;base64,"
	for name, icons := range allIcons() {
		t.Run(name, func(t *testing.T) {
			for i, entry := range []struct {
				name string
				icon mcp.Icon
			}{
				{"light", icons[1]},
				{"dark", icons[2]},
			} {
				t.Run(entry.name, func(t *testing.T) {
					payload := strings.TrimPrefix(entry.icon.Source, prefix)
					decoded, err := base64.StdEncoding.DecodeString(payload)
					if err != nil {
						t.Fatalf("entry %d: base64 decode failed: %v", i+1, err)
					}
					cfg, format, err := image.DecodeConfig(strings.NewReader(string(decoded)))
					if err != nil {
						t.Fatalf("entry %d: image.DecodeConfig failed: %v", i+1, err)
					}
					if format != "webp" {
						t.Errorf("entry %d: decoded format = %q, want %q", i+1, format, "webp")
					}
					if cfg.Width != 16 || cfg.Height != 16 {
						t.Errorf("entry %d: decoded dimensions = %dx%d, want 16x16", i+1, cfg.Width, cfg.Height)
					}
				})
			}
		})
	}
}
