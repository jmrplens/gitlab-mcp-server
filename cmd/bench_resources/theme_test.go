// theme_test.go covers reading the chart palette out of the site's stylesheet,
// including the failure the arrangement exists to catch: a token that moved,
// was renamed, or became a var() reference a chart cannot resolve.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testCSS is a stylesheet shaped like the real one: the dark scheme on the
// bare :root with ::backdrop beside it, the light scheme behind the data-theme
// attribute, and a later unrelated :root block that a naive search would run
// into.
const testCSS = `/* comment */
:root,
::backdrop {
	color-scheme: dark;
	--sl-color-gray-1: #e6edf3;
	--sl-color-gray-3: #8b949e;
	--sl-color-black: #0d1117;
	--gm-surface: #161b22;
	--gm-border: #30363d;
	--gm-mark: #a78bfa;
	--gm-mark-tip: #fc8a3d;
	--sl-color-bg: var(--sl-color-black);
}

:root[data-theme="light"],
[data-theme="light"] ::backdrop {
	color-scheme: light;
	--sl-color-gray-1: #24292f;
	--sl-color-gray-3: #59636e;
	--sl-color-black: #ffffff;
	--gm-surface: #f6f8fa;
	--gm-border: #d1d9e0;
	--gm-mark: #6d28d9;
	--gm-mark-tip: #c2410c;
}

:root {
	--gm-radius: 4px;
}
`

// TestLoadPalettes_BothSchemes_ReadTheirOwnTokens verifies each scheme is read
// from its own block, which is the whole risk: the two blocks declare the same
// property names, so a selector that matched the wrong one would paint a dark
// chart in light colors and pass every other check.
func TestLoadPalettes_BothSchemes_ReadTheirOwnTokens(t *testing.T) {
	palettes, err := loadPalettes(testCSS)
	if err != nil {
		t.Fatalf("loadPalettes: %v", err)
	}

	tests := []struct {
		scheme    string
		page      string
		text      string
		firstSeri string
	}{
		{scheme: schemeDark, page: "#0d1117", text: "#e6edf3", firstSeri: "#a78bfa"},
		{scheme: schemeLight, page: "#ffffff", text: "#24292f", firstSeri: "#6d28d9"},
	}
	for _, tc := range tests {
		t.Run(tc.scheme, func(t *testing.T) {
			p := palettes[tc.scheme]
			if p.Scheme != tc.scheme {
				t.Errorf("Scheme = %q, want %q", p.Scheme, tc.scheme)
			}
			if p.Page != tc.page {
				t.Errorf("Page = %q, want %q", p.Page, tc.page)
			}
			if p.Text != tc.text {
				t.Errorf("Text = %q, want %q", p.Text, tc.text)
			}
			if len(p.Series) != 3 {
				t.Fatalf("Series has %d colors, want 3", len(p.Series))
			}
			if p.Series[0] != tc.firstSeri {
				t.Errorf("Series[0] = %q, want %q", p.Series[0], tc.firstSeri)
			}
			if p.Series[2] != extraSeriesHue[tc.scheme] {
				t.Errorf("Series[2] = %q, want the stated third hue %q", p.Series[2], extraSeriesHue[tc.scheme])
			}
			if p.Threshold != thresholdColor[tc.scheme] {
				t.Errorf("Threshold = %q, want %q", p.Threshold, thresholdColor[tc.scheme])
			}
		})
	}
}

// TestPaletteFromTheme_MissingOrIndirect_ReturnsError verifies the two ways
// the stylesheet can stop being usable are reported: a token that is gone, and
// one that became a var() reference, which resolves in a browser and not in an
// SVG file.
func TestPaletteFromTheme_MissingOrIndirect_ReturnsError(t *testing.T) {
	tests := []struct {
		name string
		css  string
		want string
	}{
		{
			name: "no such block",
			css:  ":root[data-theme=\"light\"],\n\t--gm-mark: #fff;\n}\n",
			want: "no",
		},
		{
			name: "block never closed",
			css:  ":root,\n::backdrop {\n\t--gm-mark: #fff;\n",
			want: "not closed",
		},
		{
			name: "token removed",
			css:  strings.Replace(testCSS, "--gm-mark: #a78bfa;", "", 1),
			want: "declares no --gm-mark",
		},
		{
			name: "token is a reference",
			css:  strings.Replace(testCSS, "--gm-mark: #a78bfa;", "--gm-mark: var(--sl-color-accent);", 1),
			want: "reference",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := paletteFromTheme(tc.css, schemeDark)
			if err == nil {
				t.Fatal("paletteFromTheme accepted a stylesheet it cannot paint from")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

// TestPaletteFromTheme_UnknownScheme_ReturnsError verifies a scheme with no
// selector is refused rather than silently rendering the dark one.
func TestPaletteFromTheme_UnknownScheme_ReturnsError(t *testing.T) {
	if _, err := paletteFromTheme(testCSS, "sepia"); err == nil {
		t.Error("paletteFromTheme accepted a scheme it has no selector for")
	}
}

// TestLoadPalettes_RealStylesheet_StillParses verifies the committed site
// stylesheet is readable by this parser, so a palette change there fails here
// rather than in a chart nobody looks at closely.
func TestLoadPalettes_RealStylesheet_StillParses(t *testing.T) {
	root, err := projectRootForTest()
	if err != nil {
		t.Skipf("not running inside the module: %v", err)
	}
	css, err := os.ReadFile(filepath.Join(root, themeCSS))
	if err != nil {
		t.Fatalf("reading %s: %v", themeCSS, err)
	}
	palettes, err := loadPalettes(string(css))
	if err != nil {
		t.Fatalf("the committed stylesheet no longer yields a chart palette: %v", err)
	}
	for _, scheme := range []string{schemeLight, schemeDark} {
		t.Run(scheme, func(t *testing.T) {
			p := palettes[scheme]
			if !strings.HasPrefix(p.Page, "#") || !strings.HasPrefix(p.Series[0], "#") {
				t.Errorf("%s palette did not resolve to colors: %+v", scheme, p)
			}
		})
	}
}
