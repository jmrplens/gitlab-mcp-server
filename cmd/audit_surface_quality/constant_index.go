// The constant-index rule: a list formatter that renders the head of a slice
// where it means to render every element.

package main

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// constantIndexCategory is the rule family a constant-index violation is
// grouped under in the report.
const constantIndexCategory = "constant-index"

// constantIndexDeclarations answers a finding the rule cannot tell from a
// defect, keyed by the name the report prints for the formatter and carrying
// the reason. A declaration that answers nothing is itself a violation, so a
// fixed formatter cannot leave a stale one behind.
//
// It is empty: every registered formatter that renders a populated list
// renders each element of it.
var constantIndexDeclarations = map[string]string{}

// fixtureWalkDepth bounds the walk over a filled fixture. The filler stops at
// its own depth, so a self-referential type terminates on the nil pointer it
// left behind; this is the second bound, for a shape the filler reached by
// another route.
const fixtureWalkDepth = 8

// fixtureText spells the string one fixture field carries, for the audit's
// own renders.
//
// The shared filler spells a sentinel from the field's own name
// ([testutil.FixtureText]), and the path's slice index is dropped on the way:
// both elements of a slice of structs therefore carry the same text, and a
// formatter that reads element 0 in a loop renders exactly what a correct one
// renders. Spelling the whole path instead makes the two elements differ,
// which is what [constantIndexViolations] reads.
//
// The two shapes a render depends on are kept: an address stays an address
// under [testutil.FixtureURLBase], so a link is still a link, and an instant
// stays the RFC 3339 sentinel, so the raw-timestamp rules still see one.
func fixtureText(path string) string {
	base := testutil.FixtureText(path)
	if base == testutil.FixtureSentinelRFC3339 {
		return base
	}
	name := sanitizePath(path)
	if strings.HasPrefix(base, testutil.FixtureURLBase) {
		return testutil.FixtureURLBase + "/" + name + "/7"
	}
	return name + "7"
}

// sanitizePath keeps the letters and digits of a whole field path, so the
// sentinel holds no Markdown and names exactly one position of the fixture.
func sanitizePath(path string) string {
	var b strings.Builder
	for _, r := range path {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "Value"
	}
	return b.String()
}

// constantIndexViolations reports the formatter named by name as reading a
// constant index when its render of value carries a first element's sentinel
// twice and the second element's not at all.
//
// Twice is the condition rather than once, because a formatter that renders
// only the head of a list is a summary and not a defect: it prints the first
// element's text once and the second element's never. Printing the first
// twice while never printing the second is what a loop over a constant index
// produces.
func constantIndexViolations(name string, value reflect.Value, result *mcp.CallToolResult) []violation {
	rendered := resultText(result)
	if rendered == "" {
		return nil
	}
	var pairs [][2][]string
	collectSlicePairs(value, 0, &pairs)
	for _, pair := range pairs {
		for i, first := range pair[0] {
			second := pair[1][i]
			if first == second {
				continue
			}
			if strings.Count(rendered, first) >= 2 && !strings.Contains(rendered, second) {
				return []violation{{
					name, constantIndexCategory,
					fmt.Sprintf("the populated render carries %q twice and %q never, which is one element of a list rendered for every element", first, second),
				}}
			}
		}
	}
	return nil
}

// collectSlicePairs records, for every two-element slice the walk reaches,
// the strings its first element carries beside the strings its second
// carries. The filler gives both elements the same shape, so the two lists
// are the same length and their entries name the same position.
func collectSlicePairs(v reflect.Value, depth int, out *[][2][]string) {
	if depth > fixtureWalkDepth {
		return
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			collectSlicePairs(v.Elem(), depth+1, out)
		}
	case reflect.Struct:
		t := v.Type()
		for i := range t.NumField() {
			if !t.Field(i).IsExported() {
				continue
			}
			collectSlicePairs(v.Field(i), depth+1, out)
		}
	case reflect.Slice, reflect.Array:
		if v.Len() == 2 {
			var first, second []string
			collectStrings(v.Index(0), 0, &first)
			collectStrings(v.Index(1), 0, &second)
			if len(first) > 0 && len(first) == len(second) {
				*out = append(*out, [2][]string{first, second})
			}
		}
		for i := range v.Len() {
			collectSlicePairs(v.Index(i), depth+1, out)
		}
	default:
		// A map is left out on purpose: its entries arrive in no order, so
		// two elements' strings could not be read off as the same positions.
	}
}

// collectStrings appends the non-empty strings v carries, in field order.
func collectStrings(v reflect.Value, depth int, out *[]string) {
	if depth > fixtureWalkDepth {
		return
	}
	switch v.Kind() {
	case reflect.String:
		if s := v.String(); s != "" {
			*out = append(*out, s)
		}
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			collectStrings(v.Elem(), depth+1, out)
		}
	case reflect.Struct:
		t := v.Type()
		for i := range t.NumField() {
			if !t.Field(i).IsExported() {
				continue
			}
			collectStrings(v.Field(i), depth+1, out)
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			collectStrings(v.Index(i), depth+1, out)
		}
	default:
		// A map is left out for the reason [collectSlicePairs] states.
	}
}

// resultText is the text a client reads off a tool result, which is what the
// constant-index rule scans.
func resultText(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	var b strings.Builder
	for _, block := range result.Content {
		if text, ok := block.(*mcp.TextContent); ok {
			b.WriteString(text.Text)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// applyConstantIndexDeclarations removes the violations a declaration
// answers and reports every declaration that answered nothing, which is the
// discipline every declaration table here is held to.
func applyConstantIndexDeclarations(vs []violation) []violation {
	answered := make(map[string]bool, len(constantIndexDeclarations))
	kept := make([]violation, 0, len(vs))
	for _, v := range vs {
		if reason := constantIndexDeclarations[v.tool]; reason != "" {
			answered[v.tool] = true
			continue
		}
		kept = append(kept, v)
	}
	for _, name := range slices.Sorted(maps.Keys(constantIndexDeclarations)) {
		switch {
		case constantIndexDeclarations[name] == "":
			kept = append(kept, violation{name, constantIndexCategory, "the declaration gives no reason"})
		case !answered[name]:
			kept = append(kept, violation{name, constantIndexCategory, "the declaration answers no finding and is stale"})
		}
	}
	return kept
}
