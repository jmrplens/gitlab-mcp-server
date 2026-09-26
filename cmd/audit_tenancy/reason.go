package main

import (
	"fmt"
	"go/ast"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// checkReasons is G9: each row's quoted reason still appears, whitespace
// aside, in the doc comment of the declaration it names, or of the const or
// var block around it. A reason is what the unit it is compared in rests on,
// so a quotation that moved or changed wording under the row is the row
// silently reasoning about something the code no longer says.
func (g *gate) checkReasons() []Finding {
	var found []Finding
	for _, d := range g.reg.decisions {
		if d.Reason == "" {
			continue
		}
		if d.ReasonAt == (tenancy.Site{}) {
			found = append(found, Finding{Rule: "G9", Subject: d.ID, Message: "quotes a reason and names no declaration it is quoted from"})
			continue
		}
		decl, err := g.p.lookup(d.ReasonAt)
		if err != nil {
			continue
		}
		want := normalize(d.Reason)
		if !strings.Contains(normalize(commentText(decl.comments())), want) {
			found = append(found, Finding{
				Rule: "G9", Subject: d.ID, Position: decl.where(g.p),
				Message: fmt.Sprintf("quotes %q, which the doc comment of %s and of its block no longer say", d.Reason, decl.key),
			})
			continue
		}
		g.read.reasons++
	}
	return found
}

// commentText is the text of every non-nil comment group, joined.
func commentText(groups []*ast.CommentGroup) string {
	var parts []string
	for _, group := range groups {
		if group != nil {
			parts = append(parts, group.Text())
		}
	}
	return strings.Join(parts, "\n")
}

// normalize collapses every run of whitespace to one space, so a quotation
// survives being rewrapped.
func normalize(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
