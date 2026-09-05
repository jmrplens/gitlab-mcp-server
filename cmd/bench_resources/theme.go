// theme.go reads the chart palette out of the documentation site's stylesheet.
//
// The charts are published beside the site's own prose, so they are painted
// with the site's own tokens rather than with colors restated here. That is
// the arrangement the sibling project's perfmon charts use, and the reason is
// the same: a palette written twice drifts, and a chart that drifts reads as a
// foreign object on the page.
//
// Two colors are not in the stylesheet and are stated here, each with the
// reason it has to be: the palette carries one accent and one indicator, and a
// three-series chart needs a third hue that is neither.

package main

import (
	"fmt"
	"regexp"
	"strings"
)

// Scheme names, which double as the file-name suffix of the rendered pair.
const (
	schemeLight = "light"
	schemeDark  = "dark"
)

// palette is what a chart paints with.
type palette struct {
	Scheme string
	// Page is the figure's ground, Plot the panel the data sits on.
	Page  string
	Plot  string
	Grid  string
	Text  string
	Muted string
	// Series are the categorical colors, in the order surfaces are always
	// listed: dynamic, meta, individual.
	Series []string
	// Threshold paints the reference lines, such as the 512 MiB floor the
	// deployment documentation claims.
	Threshold string
}

// extraSeriesHue is the third categorical color, which the site palette does
// not have and a three-surface chart needs.
//
// It is the hue Starlight's `tip` family was retuned to in theme.css (190, a
// deep teal chosen there precisely because it clears the brand violet), at the
// lightness each ground needs: the values below measure above 4.5:1 on their
// own page grounds, the same floor check-contrast.mjs holds the CSS to.
var extraSeriesHue = map[string]string{
	schemeDark:  "#3fb9cc",
	schemeLight: "#0e7490",
}

// thresholdColor paints a reference line. It is deliberately not a series
// color: a dashed rule saying "512 MiB" must not be mistaken for a fourth
// measurement.
var thresholdColor = map[string]string{
	schemeDark:  "#f85149",
	schemeLight: "#b42318",
}

// themeSelectors locate each scheme's declaration block. The dark theme lives
// on the bare :root, the light one behind the data-theme attribute, which is
// the convention theme.css states at the top of each block.
var themeSelectors = map[string]string{
	schemeDark:  ":root,\n::backdrop {",
	schemeLight: ":root[data-theme=\"light\"],",
}

// loadPalettes reads both schemes out of one stylesheet.
func loadPalettes(css string) (map[string]palette, error) {
	out := make(map[string]palette, 2)
	for _, scheme := range []string{schemeLight, schemeDark} {
		p, err := paletteFromTheme(css, scheme)
		if err != nil {
			return nil, fmt.Errorf("%s palette: %w", scheme, err)
		}
		out[scheme] = p
	}
	return out, nil
}

// paletteFromTheme extracts one scheme's tokens.
func paletteFromTheme(css, scheme string) (palette, error) {
	selector, ok := themeSelectors[scheme]
	if !ok {
		return palette{}, fmt.Errorf("unknown scheme %q", scheme)
	}
	start := strings.Index(css, selector)
	if start < 0 {
		return palette{}, fmt.Errorf("theme.css: no %q block", selector)
	}
	end := strings.Index(css[start:], "\n}")
	if end < 0 {
		return palette{}, fmt.Errorf("theme.css: %q block is not closed", selector)
	}
	block := css[start : start+end]

	var err error
	read := func(name string) string {
		if err != nil {
			return ""
		}
		value, readErr := tokenValue(block, name)
		if readErr != nil {
			err = readErr
		}
		return value
	}

	p := palette{
		Scheme:    scheme,
		Page:      read("sl-color-black"),
		Plot:      read("gm-surface"),
		Grid:      read("gm-border"),
		Text:      read("sl-color-gray-1"),
		Muted:     read("sl-color-gray-3"),
		Threshold: thresholdColor[scheme],
	}
	p.Series = []string{read("gm-mark"), read("gm-mark-tip"), extraSeriesHue[scheme]}
	if err != nil {
		return palette{}, err
	}
	return p, nil
}

// tokenValue reads one custom property out of a declaration block, refusing a
// value that is itself a reference: a chart cannot resolve var().
func tokenValue(block, name string) (string, error) {
	pattern := regexp.MustCompile(`--` + regexp.QuoteMeta(name) + `:\s*([^;]+);`)
	match := pattern.FindStringSubmatch(block)
	if match == nil {
		return "", fmt.Errorf("theme.css: block declares no --%s", name)
	}
	value := strings.TrimSpace(match[1])
	if strings.HasPrefix(value, "var(") {
		return "", fmt.Errorf("theme.css: --%s is a reference (%s), which a chart cannot resolve", name, value)
	}
	return value, nil
}
