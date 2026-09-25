package main

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// reasonSource states reasons where G9 reads them: a declaration's own doc,
// the doc of the block around it, and a doc that wraps the quotation over two
// lines.
const reasonSource = siteHeader + `
// ownDoc bounds what one entry holds.
const ownDoc = 1

// The ceilings below exist because one is not enough in
// either direction.
const (
	inBlock = 2
	// noDoc has a doc of its own that does not say it.
	noDoc = 3
)
`

// reasonRow is a row quoting reason from the declaration named at.
func reasonRow(id, reason, at string) tenancy.Decision {
	d := row(id)
	d.Reason = reason
	if at != "" {
		d.ReasonAt = site(at, tenancy.Reason)
	}
	return d
}

// TestCheckReasons_AQuotationWhereTheRowSaysItIs_Passes: in the declaration's
// own doc, in its block's doc, and wrapped differently from the quotation.
func TestCheckReasons_AQuotationWhereTheRowSaysItIs_Passes(t *testing.T) {
	report := fixture{
		files: map[string]string{"site/site.go": reasonSource},
		rows: []tenancy.Decision{
			reasonRow("ROW-001", "bounds what one entry holds", "ownDoc"),
			reasonRow("ROW-002", "one is not enough in either direction", "inBlock"),
			reasonRow("ROW-003", "because one   is not enough\nin either", "noDoc"),
			row("ROW-004"),
		},
	}.run(t)
	assertFindings(t, report, "G9")
	if report.Summary.Reasons != 3 {
		t.Fatalf("reasons read = %d, want 3", report.Summary.Reasons)
	}
}

// TestCheckReasons_AQuotationThatMovedOrChanged_IsAFinding: a reason the
// comments no longer say, and a reason with no declaration to be quoted from,
// fail; a declaration that does not exist is G1's.
func TestCheckReasons_AQuotationThatMovedOrChanged_IsAFinding(t *testing.T) {
	report := fixture{
		files: map[string]string{"site/site.go": reasonSource},
		rows: []tenancy.Decision{
			reasonRow("ROW-001", "bounds fairness", "ownDoc"),
			reasonRow("ROW-002", "a reason", ""),
			reasonRow("ROW-003", "a reason", "gone"),
		},
	}.run(t)
	assertFindings(t, report, "G9",
		"ROW-001: quotes \"bounds fairness\", which the doc comment of "+siteDir+":ownDoc and of its block no longer say",
		"ROW-002: quotes a reason and names no declaration it is quoted from",
	)
}
