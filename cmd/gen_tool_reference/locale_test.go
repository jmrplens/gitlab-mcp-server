package main

import (
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
)

// emDash is U+2014, which the repository's prose does not use.
const emDash rune = 0x2014

// width is the length the site's table classifier measures a text by: one
// unit per UTF-16 code unit, which for this text is one per rune.
func width(text string) int {
	return utf8.RuneCountInString(text)
}

func TestLanguages_TableHeaders_MeasureTheSameInEnglishAndSpanish(t *testing.T) {
	pairs := []struct{ name, en, es string }{
		{"action", english.columnAction, spanish.columnAction},
		{"individual", english.columnIndividual, spanish.columnIndividual},
		{"parameter", english.columnParameter, spanish.columnParameter},
		{"type", english.columnType, spanish.columnType},
		{"mandatory", english.columnMandatory, spanish.columnMandatory},
		{"description", english.columnDesc, spanish.columnDesc},
	}
	for _, pair := range pairs {
		t.Run(pair.name, func(t *testing.T) {
			en, es := measure([][]string{{pair.en}}), measure([][]string{{pair.es}})
			if en.widths[0] != es.widths[0] || en.tokens[0] != es.tokens[0] {
				t.Errorf("%q and %q measure differently", pair.en, pair.es)
			}
		})
	}
	for _, lang := range languages {
		t.Run(lang.code+" answers", func(t *testing.T) {
			if width(lang.yes) > width(lang.columnMandatory) || width(lang.no) > width(lang.columnMandatory) {
				t.Errorf("%q or %q is wider than the header %q", lang.yes, lang.no, lang.columnMandatory)
			}
		})
		t.Run(lang.code+" tier", func(t *testing.T) {
			// The tier column is written only when its cells differ, so it
			// always holds a tier above Free, which has to outmeasure the
			// header in both languages.
			for _, tier := range []edition.Tier{edition.Premium, edition.Ultimate} {
				t.Run(tierNames[tier], func(t *testing.T) {
					if width(tierNames[tier]) < width(lang.columnTier) {
						t.Errorf("%s is narrower than the header %q", tierNames[tier], lang.columnTier)
					}
				})
			}
		})
	}
}

// tableMeasure is what the site's table classifier reads of one table: its
// column count, each column's widest cell and longest unbreakable token, the
// header row included, and the widest body cell after the first column.
type tableMeasure struct {
	columns int
	widths  []int
	tokens  []int
	body    int
}

// The renderings check-facts applies to a cell before measuring it.
var (
	cellExpression = regexp.MustCompile(`\{[^}]*\}`)
	cellCode       = regexp.MustCompile("`([^`]*)`")
	cellLink       = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	cellEmphasis   = regexp.MustCompile(`[*_]`)
	cellEscape     = regexp.MustCompile(`\\(.)`)
)

// renderCell renders a cell the way site/scripts/check-facts.mjs does.
func renderCell(cell string) string {
	cell = cellExpression.ReplaceAllString(cell, "9999")
	cell = cellCode.ReplaceAllString(cell, "$1")
	cell = cellLink.ReplaceAllString(cell, "$1")
	cell = cellEmphasis.ReplaceAllString(cell, "")
	return cellEscape.ReplaceAllString(cell, "$1")
}

// measureTables measures every pipe table of a page, splitting rows on every
// pipe as check-facts does.
func measureTables(page string) []tableMeasure {
	lines := strings.Split(page, "\n")
	var measures []tableMeasure
	for i := 0; i+1 < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "|") || !strings.HasPrefix(lines[i+1], "| -") {
			continue
		}
		var rows [][]string
		for j := i; j < len(lines) && strings.HasPrefix(lines[j], "|"); j++ {
			if j == i+1 {
				continue
			}
			var cells []string
			for cell := range strings.SplitSeq(strings.Trim(lines[j], "|"), "|") {
				cells = append(cells, renderCell(strings.TrimSpace(cell)))
			}
			rows = append(rows, cells)
			i = j
		}
		measures = append(measures, measure(rows))
	}
	return measures
}

// measure reads a table's measures from its rendered rows.
func measure(rows [][]string) tableMeasure {
	m := tableMeasure{columns: len(rows[0])}
	m.widths = make([]int, m.columns)
	m.tokens = make([]int, m.columns)
	for r, row := range rows {
		for c, cell := range row {
			m.widths[c] = max(m.widths[c], width(cell))
			for token := range strings.FieldsSeq(cell) {
				m.tokens[c] = max(m.tokens[c], width(token))
			}
			if r > 0 && c > 0 {
				m.body = max(m.body, width(cell))
			}
		}
	}
	return m
}

func TestRenderPages_RealCatalog_TablesMeasureTheSameInBothLanguages(t *testing.T) {
	data, err := decodeDomains(domainsJSON)
	if err != nil {
		t.Fatalf("decodeDomains() error = %v", err)
	}
	pages := map[string]string{}
	for _, p := range renderPages(mustRealReference(t), data) {
		pages[p.path] = string(p.content)
	}
	tables := 0
	for path, page := range pages {
		name, isEnglish := strings.CutPrefix(path, english.dir+"/")
		if !isEnglish {
			continue
		}
		en, es := measureTables(page), measureTables(pages[spanish.dir+"/"+name])
		tables += len(en)
		if !slices.EqualFunc(en, es, func(a, b tableMeasure) bool {
			return a.columns == b.columns && a.body == b.body && slices.Equal(a.widths, b.widths) && slices.Equal(a.tokens, b.tokens)
		}) {
			t.Errorf("%s: the tables measure %+v in English and %+v in Spanish", name, en, es)
		}
		if strings.ContainsRune(page, emDash) {
			t.Errorf("%s holds an em dash", name)
		}
	}
	if tables < 1000 {
		t.Errorf("measured %d tables, want the real catalog's", tables)
	}
}

func TestMeasureTables_Page_ReadsEachTable(t *testing.T) {
	page := "text\n\n| A | Bé |\n| - | -- |\n| `x` | [y z](#q) |\n| 1 | 2 |\n\nmore\n| not a table |\n"
	got := measureTables(page)
	want := tableMeasure{columns: 2, widths: []int{1, 3}, tokens: []int{1, 2}, body: 3}
	if len(got) != 1 || got[0].columns != want.columns || got[0].body != want.body ||
		!slices.Equal(got[0].widths, want.widths) || !slices.Equal(got[0].tokens, want.tokens) {
		t.Errorf("measureTables() = %+v, want [%+v]", got, want)
	}
}
