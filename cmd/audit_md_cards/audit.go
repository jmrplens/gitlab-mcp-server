package main

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/mdshape"
	_ "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools" // registers every Markdown formatter
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Finding is one block of one rendered response that does not hold together.
type Finding struct {
	Package string `json:"package"`
	Type    string `json:"type"`
	Sample  string `json:"sample"`
	Kind    string `json:"kind"`
	Line    int    `json:"line"`
	Text    string `json:"text"`
	Why     string `json:"why"`
}

// Census is the shape count for one package: how many of its responses answer
// with a card and how many with a table. It is the number issue #697 opened
// with, kept as something a run regenerates rather than something a grep
// guessed once.
type Census struct {
	Package string `json:"package"`
	Cards   int    `json:"cards"`
	Tables  int    `json:"tables"`
}

// Unrenderable is one output type the audit could not render, with the reason
// it could not. It is the audit's own blind spot, listed rather than counted
// as clean.
type Unrenderable struct {
	Type   string `json:"type"`
	Sample string `json:"sample"`
	Reason string `json:"reason"`
}

// Report is the work list one run produces.
type Report struct {
	// Rules names the shape rules this run judged, so a report and a verdict
	// both say what they are a verdict on.
	Rules         string         `json:"rules"`
	Findings      []Finding      `json:"findings"`
	Unrenderable  []Unrenderable `json:"unrenderable"`
	Census        []Census       `json:"census"`
	Types         int            `json:"types"`
	MixedPackages []string       `json:"mixed_packages"`
}

// samples names the two values rendered for every type.
var samples = []struct {
	name      string
	populated bool
}{
	{name: "zero", populated: false},
	{name: "populated", populated: true},
}

// audit renders every registered formatter and judges what comes out.
func audit() Report {
	report := Report{}
	cards := map[string]int{}
	tables := map[string]int{}

	types := toolutil.MarkdownFormatterTypes()
	sort.Slice(types, func(i, j int) bool { return types[i].String() < types[j].String() })
	report.Types = len(types)

	for _, outputType := range types {
		pkg := packageOf(outputType)
		for _, sample := range samples {
			md, err := render(outputType, sample.populated)
			if err != nil {
				report.Unrenderable = append(report.Unrenderable, Unrenderable{
					Type: outputType.String(), Sample: sample.name, Reason: err.Error(),
				})
				continue
			}
			if md == "" {
				continue
			}
			shaped := mdshape.Lint(md)
			cards[pkg] += shaped.Cards()
			tables[pkg] += shaped.Tables()
			for _, finding := range shaped.Findings {
				report.Findings = append(report.Findings, Finding{
					Package: pkg, Type: outputType.String(), Sample: sample.name,
					Kind: string(finding.Kind), Line: finding.Line, Text: finding.Text, Why: finding.Why,
				})
			}
		}
	}

	report.Census = census(cards, tables)
	for _, entry := range report.Census {
		if entry.Cards > 0 && entry.Tables > 0 {
			report.MixedPackages = append(report.MixedPackages, entry.Package)
		}
	}
	sortFindings(report.Findings)
	return report
}

// render builds a sample of the type and asks the registry for its Markdown,
// through the same entry point a tool call goes through.
//
// A formatter is written against the values GitLab sends and a sample is not
// one of them, so a panic here says the sample was wrong rather than that the
// formatter is. Recovering keeps one such type from taking the sweep down with
// it, and the type is reported.
func render(outputType reflect.Type, populated bool) (md string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			md, err = "", fmt.Errorf("formatter panicked on the sample: %v", recovered)
		}
	}()

	value := sampleValue(outputType, populated, 0)
	if !value.IsValid() {
		return "", fmt.Errorf("no sample can be built for %s", outputType)
	}
	result := toolutil.MarkdownForResult(value.Interface())
	if result == nil {
		// A formatter that returns nothing for a sample has declined to
		// render it — several return "" for an output with no identifier —
		// and a response that was never written has no shape to judge.
		return "", nil
	}
	return textOf(result), nil
}

// textOf concatenates the text content of a result, which is what the model on
// the other end reads.
func textOf(result *mcp.CallToolResult) string {
	var b strings.Builder
	for _, content := range result.Content {
		text, ok := content.(*mcp.TextContent)
		if !ok {
			continue
		}
		b.WriteString(text.Text)
	}
	return b.String()
}

// packageOf renders a type's package the way the repository names it.
func packageOf(outputType reflect.Type) string {
	path := outputType.PkgPath()
	if trimmed, ok := strings.CutPrefix(path, modulePath+"/"); ok {
		return trimmed
	}
	return path
}

// modulePath is this repository's module path, trimmed off an import path so a
// report names a package the way the repository does.
const modulePath = "github.com/jmrplens/gitlab-mcp-server/v3"

// census turns the two shape counts into one ordered list.
func census(cards, tables map[string]int) []Census {
	names := map[string]bool{}
	for pkg := range cards {
		names[pkg] = true
	}
	for pkg := range tables {
		names[pkg] = true
	}
	entries := make([]Census, 0, len(names))
	for pkg := range names {
		entries = append(entries, Census{Package: pkg, Cards: cards[pkg], Tables: tables[pkg]})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Package < entries[j].Package })
	return entries
}

// sortFindings orders the work list, so two runs over one tree produce the
// same one.
func sortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		left, right := findings[i], findings[j]
		switch {
		case left.Package != right.Package:
			return left.Package < right.Package
		case left.Type != right.Type:
			return left.Type < right.Type
		case left.Sample != right.Sample:
			return left.Sample < right.Sample
		default:
			return left.Line < right.Line
		}
	})
}
